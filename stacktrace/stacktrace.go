package stacktrace

import (
	"runtime"
	"strconv"
)

type StackTrace struct {
	Frames []StackFrame `json:"frames"`
}

type StackFrame struct {
	Function string `json:"function"`
	LineNo   string `json:"lineno"`
}

func Build(stack []uintptr) StackTrace {
	var ravenStackTrace = make([]StackFrame, len(stack))
	for i, ptr := range stack {
		var (
			f        = runtime.FuncForPC(ptr)
			funcname = "???"
			lineno   int
		)
		if f != nil {
			funcname = f.Name()
			_, lineno = f.FileLine(ptr)
		}
		ravenStackTrace[i] = StackFrame{
			Function: funcname,
			LineNo:   strconv.Itoa(lineno),
		}
	}
	return StackTrace{ravenStackTrace}
}
