package raven

import (
	"net/http"
	"os"
	"strings"

	"github.com/yext/glog"
)

var (
	projectName string
	hostname    string
)

func init() {
	hostname, _ = os.Hostname()
	if short := strings.Index(hostname, "."); short != -1 {
		hostname = hostname[:short]
	}
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
			client.Capture(fromGlogEvent(e))
		}
	}
}

// fromGlogEvent converts a glog.Event to the format expected by Sentry.
func fromGlogEvent(e glog.Event) *Event {
	message := string(e.Message)
	if square := strings.Index(message, "]"); square != -1 {
		message = message[square+1:]
	}

	eve := &Event{
		Project:    projectName,
		Level:      e.Severity,
		Message:    message,
		ServerName: hostname,
		Extra:      make(map[string]interface{}),
		StackTrace: BuildStackTrace(e.StackTrace),
		Logger:	    os.Args[0],
	}

	if line := strings.Index(eve.Message, "\n"); line != -1 {
		eve.Message = eve.Message[:line]
	}

	for _, d := range e.Data {
		switch t := d.(type) {
		case *http.Request:
			eve.Http = NewHttp(t)
			break
		default:
			//TODO(ltacon): ignore for now...
		}
	}

	eve.Extra["Source"] = sourceFromStack(eve.StackTrace)

	return eve
}

// sourceFromStack retrieves the function and line where the
// event was logged from in the format "file.Function:118".
func sourceFromStack(s StackTrace) string {
	if len(s.Frames) == 0 {
		return ""
	}

	f := s.Frames[0]
	return f.Function + ":" + f.LineNo
}
