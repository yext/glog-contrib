package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/aphistic/golf"
	"github.com/yext/glog"
	"github.com/yext/glog-contrib/stacktrace"
	"github.com/youtube/vitess/go/ratelimiter"
)

// Capture events and sends them to the gelf server.
// Events sent at a higher rate than maxEventsPerSec will be ignored.
// The uri must have a udp or tcp scheme.
func Capture(attrs map[string]interface{}, serverUri string, maxEventsPerSec int, eventCh <-chan glog.Event) error {
	c, _ := golf.NewClient()
	defer c.Close()

	if err := c.Dial(serverUri); err != nil {
		return err
	}
	logger, err := c.NewLogger()
	if err != nil {
		return err
	}

	for k, v := range attrs {
		logger.SetAttr(k, v)
	}

	rl := ratelimiter.NewRateLimiter(maxEventsPerSec, time.Second)
	for e := range eventCh {
		if !rl.Allow() {
			continue
		}

		logEvent(logger, e)
	}
	return nil
}

func logEvent(logger *golf.Logger, e glog.Event) {
	data := map[string]interface{}{}
	for _, d := range e.Data {
		switch t := d.(type) {
		case map[string]interface{}:
			for k, v := range t {
				data[k] = v
			}
		}
	}

	st := stacktrace.Build(e.StackTrace)
	var frames []string
	for _, frame := range st.Frames {
		frames = append(frames, fmt.Sprintf("function %s at line %s", frame.Function, frame.LineNo))
	}
	data["exceptionStackTrace"] = strings.Join(frames, ", ")

	message := string(e.Message)

	data["levelName"] = e.Severity
	switch e.Severity {
	case "INFO":
		logger.Infom(data, message)
	case "WARNING":
		logger.Warnm(data, message)
	case "ERROR":
		logger.Errm(data, message)
	case "FATAL":
		logger.Critm(data, message)
	}
}
