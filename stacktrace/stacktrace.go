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
	return StackTrace{ravenStackTrace}
}
