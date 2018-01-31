package stacktrace

import (
	"runtime"
	"strconv"
)

type StackTrace struct {
	Frames []StackFrame `json:"frames"`
}

type StackFrame struct {
	Filename string `json:"filename"`
	Function string `json:"function"`
	LineNo   string `json:"lineno"`
}

func Build(stack []uintptr) StackTrace {
	var ravenStackTrace = make([]StackFrame, 0, len(stack))
	frames := runtime.CallersFrames(stack)
	for {
		frame, more := frames.Next()
		ravenStackTrace = append(ravenStackTrace, StackFrame{
			Filename: frame.File,
			Function: frame.Function,
			LineNo:   strconv.Itoa(frame.Line),
		})
		if !more {
			break
		}
	}
	// Reverse the stack trace to fit with Sentry's expectations.
	for i, j := 0, len(ravenStackTrace)-1; i < j; i, j = i+1, j-1 {
		ravenStackTrace[i], ravenStackTrace[j] = ravenStackTrace[j], ravenStackTrace[i]
	}

	return StackTrace{ravenStackTrace}
}
