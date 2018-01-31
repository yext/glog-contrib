package raven

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"runtime"
	"strings"

	"github.com/yext/glog-contrib/stacktrace"
)

func NewEvent(req *http.Request, message string, depth int) *Event {
	// Keep only the first line of the error message.
	if newline := strings.Index(message, "\n"); newline != -1 {
		message = message[:newline]
	}

	var (
		callers = make([]uintptr, 20)
		written = runtime.Callers(depth, callers)
	)
	return &Event{
		Message:    message,
		Level:      "ERROR",
		StackTrace: stacktrace.Build(callers[:written]),
		Http:       NewHttp(req),
	}
}

func NewHttp(req *http.Request) *Http {
	return &Http{
		Url:         "http://" + req.Host + req.URL.Path,
		Method:      req.Method,
		Headers:     sentryHeaders(req.Header),
		Cookies:     req.Header.Get("Cookie"),
		QueryString: req.URL.RawQuery,
		Data:        sentryData(req.Body),
	}
}

type altDsn string

func AltDsn(dsn string) interface{} {
	return altDsn(dsn)
}

type fingerprint []string

// Fingerprint creates a Sentry fingerprint from a variadic set of strings.
// This fingerprint will be added to the outgoing event to allow for custom rollup.
// See: https://docs.sentry.io/learn/rollups/#customize-grouping-with-fingerprints
func Fingerprint(print ...string) interface{} {
	return fingerprint(print)
}

func sentryHeaders(headers map[string][]string) map[string]string {
	var m = map[string]string{}
	for k, v := range headers {
		// Skip including cookies in the headers.  Cookies have their own section.
		if k != "Cookie" {
			m[k] = strings.Join(v, ",")
		}
	}
	return m
}

func sentryData(body io.ReadCloser) string {
	if s, ok := body.(io.Seeker); ok {
		s.Seek(0, 0)
	}
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return fmt.Sprintf("<%v>", err)
	}
	return string(b)
}
