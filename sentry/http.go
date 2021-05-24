package sentry

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/getsentry/sentry-go"
)

// HTTP request building code, used to augment the data sent to Sentry
// if a http.Request object is passed as an argument to glog.

func buildHttpRequest(r *http.Request) *sentry.Request {
	return &sentry.Request{
		URL:         r.URL.String(),
		Method:      r.Method,
		Headers:     sentryHeaders(r.Header),
		Cookies:     r.Header.Get("Cookie"),
		QueryString: r.URL.RawQuery,
		Data:        sentryData(r.Body),
		Env:         nil,
	}
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
