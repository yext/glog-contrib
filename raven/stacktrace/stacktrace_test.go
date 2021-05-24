package stacktrace_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/yext/glog-contrib/raven/stacktrace"
)

func TestBuild(t *testing.T) {
	var (
		callers = make([]uintptr, 20)
		written = runtime.Callers(1, callers)
	)
	trace := stacktrace.Build(callers[:written])
	if len(trace.Frames) == 0 {
		t.Error("Expected at least one frame")
		return
	}
	if !strings.Contains(trace.Frames[len(trace.Frames)-1].Filename, "stacktrace_test.go") {
		t.Log("Stack trace returned:")
		for _, frame := range trace.Frames {
			t.Log(frame.Filename)
		}
		t.Error("Final line of stacktrace was not as expected")
	}
	for _, frame := range trace.Frames {
		if frame.AbsPath == frame.Filename {
			t.Errorf("Did not expect absolute path to match filename: %+v", frame)
		}
	}
}
