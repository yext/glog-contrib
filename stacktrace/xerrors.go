package stacktrace

import (
	"golang.org/x/xerrors"

	"github.com/getsentry/sentry-go"
	"github.com/yext/glog"
)

// GetXErrorStackTrace returns a combined stack trace incorporating the stack of
// the logging call site and that of the error it's logging.
func GetXErrorStackTrace(callSite sentry.Stacktrace, err error) *sentry.Stacktrace {
	xs := &xerrorsStack{trace: callSite}
	for err != nil {
		xs.detail = false
		switch xerr := err.(type) {
		case xerrors.Formatter:
			err = xerr.FormatError(xs)
		case xerrors.Wrapper:
			err = xerr.Unwrap()
		default:
			err = nil
		}
	}
	return &xs.trace
}

// xerrorsStack implements xerrors.Printer to capture only the wrapped stack trace.
//
// Exploits the fact that xerrors.Frame is always written as detail (and nothing else is, for any
// known implementation).
//
// It expects a sequence of alternating calls like this:
//
//   Printf("%s\n    ", []interface {}{"package.FuncName"})
//   Printf("%s:%d\n", []interface {}{"/absolute/path/to/file.go", 47})
type xerrorsStack struct {
	detail bool
	trace  sentry.Stacktrace
	fnName string
}

func (x *xerrorsStack) Print(args ...interface{}) {}

func (x *xerrorsStack) Printf(format string, args ...interface{}) {
	if x.detail {
		switch len(args) {
		case 1:
			if fn, ok := args[0].(string); ok {
				x.fnName = fn
			}
		case 2:
			var (
				absPath, ok1 = args[0].(string)
				lineno, ok2  = args[1].(int)
			)
			if !ok1 || !ok2 {
				glog.Warningf("unexpected: Printf(%q, %#v)", format, args)
				return
			}
			// fixUpFrame will clean up the Filename/AbsPath
			x.trace.Frames = append(x.trace.Frames, fixUpFrame(sentry.Frame{
				AbsPath:  absPath,
				Filename: absPath,
				Function: x.fnName,
				Lineno:   lineno,
			}))
		}
	}
}

func (x *xerrorsStack) Detail() bool {
	x.detail = true
	return true
}
