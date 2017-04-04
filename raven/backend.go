package raven

import (
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/yext/glog"
	"github.com/yext/glog-contrib/stacktrace"
)

var (
	projectName string
	hostname    string
	re          *regexp.Regexp
)

func init() {
	hostname, _ = os.Hostname()
	if short := strings.Index(hostname, "."); short != -1 {
		hostname = hostname[:short]
	}
	re = regexp.MustCompile("[0-9]{2,}")
}

// CaptureErrors sets the name of the project so that when events are
// sent to sentry they are tagged as coming from the given
// project. It then sets up the connection to sentry and begins
// to send any errors recieved over comm to sentry.
// It panics if a client could be initialized.
func CaptureErrors(project, dsn string, comm <-chan glog.Event) {
	projectName = project
	client, err := NewClient(dsn)
	if err != nil {
		panic(err)
	}

	for e := range comm {
		if e.Severity == "ERROR" {
			client.CaptureGlogEvent(e)
		}
	}
}

func separateMessageAndIds(message string) (string, string) {
	msg := re.ReplaceAllString(message, "[ID]")
	numbers := re.FindAllString(message, -1)
	ids := strings.Join(numbers, " ")
	return msg, ids
}

// fromGlogEvent converts a glog.Event to the format expected by Sentry.
func fromGlogEvent(e glog.Event) *Event {
	message := string(e.Message)
	if square := strings.Index(message, "]"); square != -1 {
		message = message[square+1:]
	}

	fullMessage := message
	if line := strings.Index(message, "\n"); line != -1 {
		message = message[:line]
	}

	msg, ids := separateMessageAndIds(message)

	eve := &Event{
		Project:    projectName,
		Level:      e.Severity,
		Message:    msg,
		ServerName: hostname,
		Extra:      map[string]interface{}{"FullMessage": fullMessage},
		StackTrace: stacktrace.Build(e.StackTrace),
		Logger:     os.Args[0],
	}

	data := map[string]interface{}{}
	for _, d := range e.Data {
		switch t := d.(type) {
		case *http.Request:
			eve.Http = NewHttp(t)
		case map[string]interface{}:
			for k, v := range t {
				data[k] = v
			}
		default:
			//TODO(ltacon): ignore for now...
		}
	}

	eve.Extra["Data"] = data
	eve.Extra["Source"] = sourceFromStack(eve.StackTrace)
	if ids != "" {
		eve.Extra["IDs"] = ids
	}

	return eve
}

// sourceFromStack retrieves the function and line where the
// event was logged from in the format "file.Function:118".
func sourceFromStack(s stacktrace.StackTrace) string {
	if len(s.Frames) == 0 {
		return ""
	}

	f := s.Frames[0]
	return f.Function + ":" + f.LineNo
}
