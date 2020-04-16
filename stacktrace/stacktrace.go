package stacktrace

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type StackTrace struct {
	Frames []StackFrame `json:"frames"`
}

type StackFrame struct {
	AbsPath  string `json:"abs_path"`
	Filename string `json:"filename"`
	Function string `json:"function"`
	LineNo   string `json:"lineno"`
}

func Build(stack []uintptr) StackTrace {
	var ravenStackTrace = make([]StackFrame, 0, len(stack))
	frames := runtime.CallersFrames(stack)
	for {
		frame, more := frames.Next()

		absPath := frame.File
		file := filepath.Base(absPath)

		// Sanitize the path to remove GOPATH and obtain the import path.
		// Will take the path after the last instance of '/src/'.
		// This may omit some of the path if there is an src directory in a package import path.
		candidates := strings.SplitAfter(absPath, "/src/")
		if len(candidates) > 0 {
			file = candidates[len(candidates)-1]
		}

		ravenStackTrace = append(ravenStackTrace, StackFrame{
			AbsPath:  absPath,
			Filename: file,
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

// Inner returns the innermost stack frame.
func (st StackTrace) Inner() StackFrame {
	if len(st.Frames) == 0 {
		return StackFrame{}
	}
	return st.Frames[len(st.Frames)-1]
}

// Strings returns a list of string descriptions of each stack frame.
func (st StackTrace) Strings() []string {
	var r = make([]string, len(st.Frames))
	for i, f := range st.Frames {
		r[i] = fmt.Sprintf("%s in %s at line %s", f.Filename, f.Function, f.LineNo)
	}
	return r
}
