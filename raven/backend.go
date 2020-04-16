package raven

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/yext/glog"
	"github.com/yext/glog-contrib/stacktrace"
	"golang.org/x/xerrors"
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
// It panics if a client could not be initialized.
func CaptureErrors(project, dsn string, comm <-chan glog.Event) {
	projectName = project
	client, err := NewClient(dsn)
	if err != nil {
		panic(err)
	}

	for glogEve := range comm {
		if glogEve.Severity == "ERROR" {
			client.CaptureGlogEvent(glogEve)
		}
	}
}

// CaptureErrorsAltDsn allows you to have errors sent to one of multiple dsn targes.
// It sets up a connection to sentry for each of the given dsn URIs.
//
// To tag an event with a dsn:
//     glog.Error("bad thing happened", glog.Data(raven.AltDsn(YOUR_DSN)))
//
// If the dsn of an event is not specified or is not equal to any of the
// dsns arg, the dsn target will be assumed to be the first dsn in the dsns list.
func CaptureErrorsAltDsn(project string, dsns []string, comm <-chan glog.Event) {
	if len(dsns) == 0 {
		panic("must specify at least one dsn")
	}
	projectName = project

	var primaryClient *Client
	dsnClients := make(map[string]*Client)
	for _, dsn := range dsns {
		client, err := NewClient(dsn)
		if err != nil {
			panic(err)
		}
		if primaryClient == nil {
			primaryClient = client
		}
		dsnClients[dsn] = client
	}

	for glogEve := range comm {
		if glogEve.Severity == "ERROR" {
			e := fromGlogEvent(glogEve)
			eventDsnTarget := e.TargetDsn
			if client, ok := dsnClients[eventDsnTarget]; ok {
				client.Capture(e)
			} else {
				primaryClient.Capture(e)
			}
		}
	}
}

// fromGlogEvent converts a glog.Event to the format expected by Sentry.
func fromGlogEvent(e glog.Event) *Event {
	message := string(e.Message)
	if square := strings.Index(message, "] "); square != -1 {
		message = message[square+2:]
	}

	logtrace := stacktrace.Build(e.StackTrace)
	eve := &Event{
		Project:    projectName,
		Level:      strings.ToLower(e.Severity),
		Message:    message,
		ServerName: hostname,
		Extra: map[string]interface{}{
			"Source": sourceFromStack(logtrace),
		},
		StackTrace: logtrace,
		Logger:     os.Args[0],
	}

	data := map[string]interface{}{}
	for _, d := range e.Data {
		switch t := d.(type) {
		case altDsn:
			eve.TargetDsn = string(d.(altDsn))
		case fingerprint:
			eve.Fingerprint = []string(d.(fingerprint))
		case *http.Request:
			eve.Http = NewHttp(t)
		case map[string]interface{}:
			for k, v := range t {
				data[k] = v
			}
		case glog.ErrorArg:
			// Prepend the Message with the innermost error message.
			// This causes it to be used for the headline.
			eve.Message = headline(t.Error) + "\n\n" + message

			// Augment the stack trace of the call site with the stack trace in
			// the error.
			eve.StackTrace = getXErrorStackTrace(eve.StackTrace, t.Error)
		default:
			//TODO(ltacon): ignore for now...
		}
	}

	// By default, set the fingerprint based on the stack trace.
	// Sentry is supposed to do that by default, but it does not appear to work.
	if len(eve.Fingerprint) == 0 {
		eve.Fingerprint = eve.StackTrace.Strings()
	}

	if len(data) > 0 {
		eve.Extra["Data"] = data
	}

	return eve
}

// sourceFromStack retrieves the function and line where the
// event was logged from in the format "file.Function:118".
func sourceFromStack(s stacktrace.StackTrace) string {
	if len(s.Frames) == 0 {
		return ""
	}

	f := s.Inner()
	return f.Function + ":" + f.LineNo
}

// headline returns a good headline for this error.
// Ideally, it returns a succinct summary that best conveys the error.
// Most likely, that's something close to the root cause, but that may
// be something boring like "context canceled".
func headline(err error) string {
	// Heuristic: return the error message from the second innermost error.
	// This provides context on the error, since returned errors are often constants.
	var prev error
	for {
		wrapper, ok := err.(xerrors.Wrapper)
		if !ok {
			break
		}
		prev = err
		err = wrapper.Unwrap()
	}
	if prev != nil {
		return prev.Error()
	}
	return err.Error()
}

// getXErrorStackTrace returns a combined stack trace incorporating the stack of
// the logging call site and that of the error it's logging.
func getXErrorStackTrace(callSite stacktrace.StackTrace, err error) stacktrace.StackTrace {
	xs := &xerrorsStack{trace: callSite}
	for err != nil {
		xs.detail = false
		switch xerr := err.(type) {
		case xerrors.Formatter:
			err = xerr.FormatError(xs)
		case xerrors.Wrapper:
			err = xerr.Unwrap()
		default:
			err = nil
		}
	}
	return xs.trace
}

// xerrorsStack implements xerrors.Printer to capture only the wrapped stack trace.
//
// Exploits the fact that xerrors.Frame is always written as detail (and nothing else is, for any
// known implementation).
//
// It expects a sequence of alternating calls like this:
//
//   Printf("%s\n    ", []interface {}{"package.FuncName"})
//   Printf("%s:%d\n", []interface {}{"/absolute/path/to/file.go", 47})
type xerrorsStack struct {
	detail bool
	trace  stacktrace.StackTrace
	fnName string
}

func (x *xerrorsStack) Print(args ...interface{}) {}

func (x *xerrorsStack) Printf(format string, args ...interface{}) {
	if x.detail {
		switch len(args) {
		case 1:
			if fn, ok := args[0].(string); ok {
				x.fnName = fn
			}
		case 2:
			var (
				absPath, ok1 = args[0].(string)
				lineno, ok2  = args[1].(int)
			)
			if !ok1 || !ok2 {
				glog.Warningf("unexpected: Printf(%q, %#v)", format, args)
				return
			}
			x.trace.Frames = append(x.trace.Frames, stacktrace.StackFrame{
				AbsPath:  absPath,
				Filename: gopathRelativeFile(absPath),
				Function: x.fnName,
				LineNo:   strconv.Itoa(lineno),
			})
		}
	}
}

func (x *xerrorsStack) Detail() bool {
	x.detail = true
	return true
}

// gopathRelativeFile sanitizes the path to remove GOPATH and obtain the import path.
// Concretely, this takes the path after the last instance of '/src/'.
// This may omit some of the path if there is an src directory in a package import path.
// If there are no /src/ directories in the path, the base filename is returned.
func gopathRelativeFile(absPath string) string {
	candidates := strings.SplitAfter(absPath, "/src/")
	if len(candidates) > 0 {
		return candidates[len(candidates)-1]
	}
	return filepath.Base(absPath)
}
