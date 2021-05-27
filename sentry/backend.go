// The Sentry package contains an implementation of a bridge between
// the sentry-go package and glog which allows for glog errors of
// level ERROR to be tracked as errors in Sentry. This includes
// custom code that interfaces with the xerrors package (as well
// as the Yext fork named yerrors) in order to send stack trace
// data for the glog invocation, error object construction,
// and any masked error calls to Sentry.
package sentry

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/yext/glog"
	"github.com/yext/glog-contrib/stacktrace"
)

// The maximum number of wrapped errors processed.
const maxErrorDepth = 10

var (
	sentryDebug = flag.Bool("sentryDebug", false,
		"enable debug mode in Sentry clients")
	sentryFingerprinting = flag.Bool("sentryFingerprinting", false,
		"enable server-side issue fingerprinting. If set, duplicate issues will only be tracked if they have equivalent filenames and line numbers")

	hostname string
)

func init() {
	hostname, _ = os.Hostname()
	if short := strings.Index(hostname, "."); short != -1 {
		hostname = hostname[:short]
	}
}

// CaptureErrors is the entrypoint for tracking Sentry exceptions via glog.
// Given Sentry DSNs and client options (DSN should not be specified in opts),
// constructs individual Sentry Client's for each DSN. The glog.Event channel
// should be provided by running glog.RegisterBackend(). For example:
//  sentry.CaptureErrors(
//  	"projectName",
//  	[]string{"https://primaryDsn", "https://optionalSecondaryDsn", ...},
//		sentrygo.ClientOptions{
//			Release: "release",
//			Environment: "prod",
//		},
//		glog.RegisterBackend())
//
// When an event is received via glog at the ERROR severity,
// the first provided DSN will be used, unless a sentry.AltDsn is
// tagged on the glog event, in which case the specified client
// for that DSN will be used:
//   glog.Error("error for secondary DSN", sentry.AltDsn("https://optionalSecondaryDsn"))
func CaptureErrors(project string, dsns []string, opts sentry.ClientOptions, comm <-chan glog.Event) {
	// If no DSNs specified, panic (we can't invoke glog)
	if len(dsns) == 0 {
		panic("must specify at least one Sentry DSN")
	}

	hubs := make(map[string]*sentry.Hub)
	var primaryHub *sentry.Hub
	for _, dsn := range dsns {
		client, err := sentry.NewClient(buildClientOptions(dsn, opts))

		// If unable to initialize the Sentry client, panic (we can't invoke glog)
		if err != nil {
			panic(err)
		}

		// Initialize a Hub (which contains additional scope)
		scope := sentry.NewScope()
		hub := sentry.NewHub(client, scope)

		// Set the first provided DSN as the primary hub
		if primaryHub == nil {
			primaryHub = hub
		}

		// Configure the cleanup period for the newly initialized client
		defer client.Flush(1 * time.Second)

		hubs[dsn] = hub
	}

	// This for loop runs indefinitely unless the glog channel closes
	// (which should only happen on app exit)
	for glogEvent := range comm {
		if glogEvent.Severity == "ERROR" {
			e, targetDsn := FromGlogEvent(glogEvent)
			if hub, ok := hubs[targetDsn]; ok {
				hub.CaptureEvent(e)
			} else {
				primaryHub.CaptureEvent(e)
			}
		}
	}
}

// Adds the dsn, server hostname, and debug status to the provided client options
func buildClientOptions(dsn string, opts sentry.ClientOptions) sentry.ClientOptions {
	opts.Dsn = dsn
	if !opts.Debug {
		opts.Debug = *sentryDebug
	}
	opts.ServerName = hostname

	return opts
}

// Builds a fingerprint of the filename, function, and line number for all
// of the frames in the top (most important) exception stacktrace.
func buildFingerprint(exceptions []sentry.Exception) []string {
	var r []string
	ex := exceptions[0]
	for _, f := range ex.Stacktrace.Frames {
		if f.InApp {
			r = append(r, fmt.Sprintf("%s in %s at line %d", f.Filename, f.Function, f.Lineno))
		}
	}
	return r
}

// FromGlogEvent processes a glog event and generates a corresponding Sentry event.
// This includes building the stacktrace, cleaning up the error title and subtitle,
// and identifying whether any TargetDSN or Fingerprint overrides were set.
func FromGlogEvent(e glog.Event) (*sentry.Event, string) {
	targetDsn := ""

	s := sentry.NewEvent()
	s.Message = removeGlogPrefixFromMessage(e.Message)
	s.Level = buildLevel(e.Severity)
	s.ServerName = hostname

	s.Extra = map[string]interface{}{}
	s.Logger = stacktrace.GopathRelativeFile(os.Args[0])

	data := map[string]interface{}{}
	sanitizedFormatString := ""
	for _, d := range e.Data {
		switch t := d.(type) {
		case altDsn:
			targetDsn = string(d.(altDsn))
		case fingerprint:
			s.Fingerprint = []string(d.(fingerprint))
		case *http.Request:
			s.Request = buildHttpRequest(t)
		case map[string]interface{}:
			for k, v := range t {
				data[k] = v
			}
		case glog.FormatStringArg:
			// If we have a format string arg, then we can use it
			// to make a rough approximation of the error's "type"
			// by removing the format characters (like %s).
			sanitizedFormatString = cleanupFormatString(t.Format)
		case glog.ErrorArg:
			// Prepend the Message with the innermost error message.
			// This causes it to be used for the headline.
			hl := headline(t.Error)
			s.Message = prependMessage(hl, s.Message)

			// Augment the stack trace of the call site with the stack trace in
			// the error. Loop through and unwrap any chained errors.
			err := t.Error
			for i := 0; i < maxErrorDepth && err != nil; i++ {
				errTrace := stacktrace.ExtractStacktrace(err)
				fullMsg := prependMessage(headline(err), err.Error())

				// Split the message into parts before and after the colon (:),
				// if one is present. This removes most unique identifiers from
				// the type field of the exception.
				msgType, msgValue := splitMessage(fullMsg)
				s.Exception = append(s.Exception, sentry.Exception{
					// Type is the bolded, primary issue title containing the primary component of the error string.
					// it is utilized in Sentry's event-merge algorithm, so we attempt to remove any potentially
					// unique components and move them over to the value field.
					Type: msgType,
					// Value is the issue subtitle containing any remaining components of the error string,
					// and the method name/line in which this error was invoked
					Value:      addExceptionSource(msgValue, errTrace),
					Stacktrace: errTrace,
				})
				switch previous := err.(type) {
				case interface{ Unwrap() error }:
					err = previous.Unwrap()
				case interface{ Cause() error }:
					err = previous.Cause()
				default:
					err = nil
				}
			}
		default:
			// ignored
		}
	}

	// Append the stacktrace provided by glog as the top Exception object,
	// since it provides information about when glog was invoked in the code
	trace := stacktrace.ExtractFrames(e.StackTrace, nil)
	if trace != nil {
		// Add exception for top-level glog message, if we did not find any
		// stacktrace data via ErrorArgs.
		var msgType, msgValue string

		// If a format string was passed to glog, use a sanitized version of it
		// as the exception type, since we know with relative certainty that it
		// will not contain any unique identifiers.
		if sanitizedFormatString != "" {
			msgType = sanitizedFormatString
			_, msgValue = splitMessage(s.Message)
		} else {
			msgType, msgValue = splitMessage(s.Message)
		}

		s.Exception = append(s.Exception, sentry.Exception{
			// Type is the primary issue title containing the primary component of the error string.
			// it is utilized in Sentry's event-merge algorithm, so we attempt to remove any potentially
			// unique components and move them over to the value field.
			Type: msgType,
			// Value is the issue subtitle containing any remaining components of the error string,
			// and the method name/line in which this error was invoked
			Value:      addExceptionSource(msgValue, trace),
			Stacktrace: trace,
		})
	}

	// Reverse the order of the Exception array
	reverse(s.Exception)

	// Set the fingerprint based on the stack trace, if option is specified.
	// This overrides logic in Sentry which will take the specific error
	// message in to account. It instead will be identified by the filename,
	// method name, and line number.
	if len(s.Fingerprint) == 0 && *sentryFingerprinting {
		s.Fingerprint = buildFingerprint(s.Exception)
	}

	if len(data) > 0 {
		s.Extra["Data"] = data
	}

	return s, targetDsn
}

func reverse(e []sentry.Exception) {
	for i := len(e)/2 - 1; i >= 0; i-- {
		o := len(e) - 1 - i
		e[i], e[o] = e[o], e[i]
	}
}
