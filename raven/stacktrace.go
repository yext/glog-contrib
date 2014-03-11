package raven

import (
	"runtime"
	"strconv"
)

func BuildStackTrace(stack []uintptr) StackTrace {
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
