/*
  Forked from github.com/kisielk/raven-go at revision
  1833b9bb1f80ff05746875be4361b52a00c50952

	Package raven is a client and library for sending messages and exceptions to Sentry: http://getsentry.com

	Usage:

	Create a new client using the NewClient() function. The value for the DSN parameter can be obtained
	from the project page in the Sentry web interface. After the client has been created use the CaptureMessage
	method to send messages to the server.

		client, err := sentry.NewClient(dsn)
		...
		id, err := client.CaptureMessage("some text")

	If you want to have more finegrained control over the send event, you can create the event instance yourself

		client.Capture(&sentry.Event{Message: "Some Text", Logger:"auth"})

*/
package raven

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/yext/glog"
	"github.com/yext/glog-contrib/raven/stacktrace"
)

type Client struct {
	URL        *url.URL
	PublicKey  string
	SecretKey  string
	Project    string
	httpClient *http.Client
	Tags       map[string]string
}

type Http struct {
	Url         string            `json:"url"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers"`
	Cookies     string            `json:"cookies"`
	Data        interface{}       `json:"data"`
	QueryString string            `json:"query_string"`
}

type Event struct {
	EventId     string                 `json:"event_id"`
	Project     string                 `json:"project"`
	Message     string                 `json:"message"`
	Timestamp   string                 `json:"timestamp"`
	Level       string                 `json:"level"`
	Logger      string                 `json:"logger"`
	ServerName  string                 `json:"server_name"`
	StackTrace  stacktrace.StackTrace  `json:"stacktrace"`
	Http        *Http                  `json:"request"`
	TargetDsn   string                 `json:"targetDsn"`
	Extra       map[string]interface{} `json:"extra"`
	Tags        map[string]string      `json:"tags"`
	Fingerprint []string               `json:"fingerprint,omitempty"`
}

type sentryResponse struct {
	ResultId string `json:"result_id"`
}

// Default sentry DSN from https://github.com/getsentry/sentry-java/blob/af5196bd2a2531d4a3a74b51aeb64319c82c4ef6/sentry/src/main/java/io/sentry/dsn/Dsn.java#L20
const DefaultSentryDSN = "noop://user:password@localhost:0/0"

// Template for the X-Sentry-Auth header
const xSentryAuthTemplate = "Sentry sentry_version=2.0, sentry_client=raven-go/0.1, sentry_timestamp=%v, sentry_key=%v"

// An iso8601 timestamp without the timezone. This is the format Sentry expects.
const iso8601 = "2006-01-02T15:04:05"

// NewClient creates a new client for a server identified by the given dsn
// A dsn is a string in the form:
//	{PROTOCOL}://{PUBLIC_KEY}:{SECRET_KEY}@{HOST}/{PATH}{PROJECT_ID}
// eg:
//	http://abcd:efgh@sentry.example.com/sentry/project1
func NewClient(dsn string) (client *Client, err error) {
	// sentry-go supports a blank DSN as a noop host. Ensure that
	// if a blank DSN is specified to raven that we treat it like
	// the default DSN.
	if dsn == "" {
		dsn = DefaultSentryDSN
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return nil, err
	}

	basePath := path.Dir(u.Path)
	project := path.Base(u.Path)

	if u.User == nil {
		return nil, fmt.Errorf("the DSN must contain a public and secret key")
	}
	publicKey := u.User.Username()
	secretKey, keyIsSet := u.User.Password()
	if !keyIsSet {
		return nil, fmt.Errorf("the DSN must contain a secret key")
	}

	u.Path = basePath

	check := func(req *http.Request, via []*http.Request) error {
		fmt.Printf("%+v", req)
		return nil
	}
	m := make(map[string]string)
	if os.Getenv("KHAN_JOB_NAME") != "" {
		m["job_name"] = strings.ToLower(os.Getenv("KHAN_JOB_NAME"))
	} else {
		m["job_name"] = "unknown"
	}

	if yextSite := os.Getenv("YEXT_SITE"); yextSite != "" {
		m["environment"] = yextSite
	}

	return &Client{
		URL:       u,
		PublicKey: publicKey,
		SecretKey: secretKey,
		httpClient: &http.Client{
			Transport:     nil,
			CheckRedirect: check,
			Jar:           nil,
		},
		Project: project,
		Tags:    m,
	}, nil
}

// CaptureMessage sends a message to the Sentry server. The resulting string is an event identifier.
func (client Client) CaptureMessage(message ...string) (result string, err error) {
	ev := Event{Message: strings.Join(message, " ")}
	sentryErr := client.Capture(&ev)

	if sentryErr != nil {
		return "", sentryErr
	}
	return ev.EventId, nil
}

// CaptureMessagef is similar to CaptureMessage except it is using Printf like parameters for
// formatting the message
func (client Client) CaptureMessagef(format string, a ...interface{}) (result string, err error) {
	return client.CaptureMessage(fmt.Sprintf(format, a...))
}

func (client Client) CaptureGlogEvent(ev glog.Event) {
	if err := client.Capture(fromGlogEvent(ev)); err != nil {
		// Don't use glog, or we'll just end up in an infinite loop
		log.Printf("Error sending error to Sentry:\n%v for glog event with message: %s, data: %v",
			err, string(ev.Message), ev.Data)
	}
}

// Sends the given event to the sentry servers after encoding it into a byte slice.
func (client Client) Capture(ev *Event) error {
	// Fill in defaults
	ev.Project = client.Project
	if ev.EventId == "" {
		eventId, err := uuid4()
		if err != nil {
			return err
		}
		ev.EventId = eventId
	}
	if ev.Level == "" {
		ev.Level = "error"
	}
	if ev.Logger == "" {
		ev.Logger = "root"
	}
	if ev.Timestamp == "" {
		now := time.Now().UTC()
		ev.Timestamp = now.Format(iso8601)
	}

	if ev.Tags == nil {
		ev.Tags = client.Tags
	} else {
		// Include any tags from the client
		for key, val := range client.Tags {
			_, exists := ev.Tags[key]
			if !exists {
				ev.Tags[key] = val
			}
		}
	}

	// Send
	timestamp, err := time.Parse(iso8601, ev.Timestamp)
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)
	b64Encoder := base64.NewEncoder(base64.StdEncoding, buf)
	writer := zlib.NewWriter(b64Encoder)
	jsonEncoder := json.NewEncoder(writer)

	if err := jsonEncoder.Encode(ev); err != nil {
		return err
	}

	err = writer.Close()
	if err != nil {
		return err
	}

	err = b64Encoder.Close()
	if err != nil {
		return err
	}

	err = client.send(buf.Bytes(), timestamp)
	if err != nil {
		return err
	}

	return nil
}

// sends a packet to the sentry server with a given timestamp
func (client Client) send(packet []byte, timestamp time.Time) (err error) {
	apiURL := *client.URL
	apiURL.Path = path.Join(apiURL.Path, "/api/"+client.Project+"/store")
	apiURL.Path += "/"
	location := apiURL.String()

	// for loop to follow redirects
	for {
		buf := bytes.NewBuffer(packet)
		req, err := http.NewRequest("POST", location, buf)
		if err != nil {
			return err
		}

		authHeader := fmt.Sprintf(xSentryAuthTemplate, timestamp.Unix(), client.PublicKey)
		req.Header.Add("X-Sentry-Auth", authHeader)
		req.Header.Add("Content-Type", "application/octet-stream")
		req.Header.Add("Connection", "close")
		req.Header.Add("Accept-Encoding", "identity")

		resp, err := client.httpClient.Do(req)

		if err != nil {
			return err
		}

		defer resp.Body.Close()

		switch resp.StatusCode {
		case 301:
			// set the location to the new one to retry on the next iteration
			location = resp.Header["Location"][0]
		case 200:
			return nil
		default:
			return errors.New(resp.Status)
		}
	}
	// should never get here
	panic("send broke out of loop")
}

func uuid4() (string, error) {
	//TODO: Verify this algorithm or use an external library
	uuid := make([]byte, 16)
	n, err := rand.Read(uuid)
	if n != len(uuid) || err != nil {
		return "", err
	}
	uuid[8] = 0x80
	uuid[4] = 0x40

	return hex.EncodeToString(uuid), nil
}
