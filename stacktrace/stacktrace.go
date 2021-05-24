package stacktrace

import (
	"reflect"
	"runtime"
	"strings"

	"github.com/getsentry/sentry-go"
)

// The following methods are taken from stacktrace.go in the sentry-go package
// because non-exported methods are needed to process frames from a glog message
// without wrapping it in an error (which prevents augmenting with xerrors info).
//
// Copyright (c) 2019 Sentry (https://sentry.io) and individual contributors.
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:
//
// * Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.
// * Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

// ExtractStacktrace creates a new Stacktrace based on the given error.
func ExtractStacktrace(err error) *sentry.Stacktrace {
	method := extractReflectedStacktraceMethod(err)

	var pcs []uintptr

	if method.IsValid() {
		pcs = extractPcs(method)
	} else {
		pcs = extractXErrorsPC(err)
	}

	if len(pcs) == 0 {
		return nil
	}

	// PATCH(jwoglom): split out extracting frames to new method
	return ExtractFrames(pcs, err)
}

// PATCH(jwoglom): ExtractFrames is split-out code from ExtractStacktrace
// which allows for frame extraction to be performed given glog PC pointers.
// The err argument is optional, if it is nil augmentation of xerror info is
// not provided.
func ExtractFrames(pcs []uintptr, err error) *sentry.Stacktrace {
	frames := extractFrames(pcs)
	frames = filterFrames(frames)

	stacktrace := sentry.Stacktrace{
		Frames: frames,
	}

	// augment with xerrors stack trace, if available
	return GetXErrorStackTrace(stacktrace, err)
}

// NewFrame assembles a stacktrace frame out of runtime.Frame.
func NewFrame(f runtime.Frame) sentry.Frame {
	// PATCH(jwoglom): wrap the exported NewFrame method
	// with some custom handling

	return fixUpFrame(sentry.NewFrame(f))
}

// PATCH(jwoglom): fixes up the given frame
func fixUpFrame(frame sentry.Frame) sentry.Frame {
	// Without an absolute filesystem path for AbsPath,
	// Sentry will not pull in neighboring code segments.
	if frame.AbsPath != "" && !strings.HasPrefix(frame.AbsPath, "/") {
		frame.AbsPath = GuessAbsPath(frame.AbsPath)
	} else if frame.AbsPath == "" {
		frame.AbsPath = GuessAbsPath(frame.Filename)
	}

	// Clean up the returned filename to remove the gopath
	frame.Filename = GopathRelativeFile(frame.Filename)

	return frame
}

func extractFrames(pcs []uintptr) []sentry.Frame {
	var frames []sentry.Frame
	callersFrames := runtime.CallersFrames(pcs)

	for {
		callerFrame, more := callersFrames.Next()

		frames = append([]sentry.Frame{
			NewFrame(callerFrame),
		}, frames...)

		if !more {
			break
		}
	}

	return frames
}

// filterFrames filters out stack frames that are not meant to be reported to
// Sentry. Those are frames internal to the SDK or Go.
func filterFrames(frames []sentry.Frame) []sentry.Frame {
	if len(frames) == 0 {
		return nil
	}

	filteredFrames := make([]sentry.Frame, 0, len(frames))

	for _, frame := range frames {
		// Skip Go internal frames.
		if frame.Module == "runtime" || frame.Module == "testing" {
			continue
		}
		// Skip Sentry internal frames, except for frames in _test packages (for
		// testing).
		if strings.HasPrefix(frame.Module, "github.com/getsentry/sentry-go") &&
			!strings.HasSuffix(frame.Module, "_test") {
			continue
		}
		filteredFrames = append(filteredFrames, frame)
	}

	return filteredFrames
}

func extractReflectedStacktraceMethod(err error) reflect.Value {
	var method reflect.Value

	// https://github.com/pingcap/errors
	methodGetStackTracer := reflect.ValueOf(err).MethodByName("GetStackTracer")
	// https://github.com/pkg/errors
	methodStackTrace := reflect.ValueOf(err).MethodByName("StackTrace")
	// https://github.com/go-errors/errors
	methodStackFrames := reflect.ValueOf(err).MethodByName("StackFrames")

	if methodGetStackTracer.IsValid() {
		stacktracer := methodGetStackTracer.Call(make([]reflect.Value, 0))[0]
		stacktracerStackTrace := reflect.ValueOf(stacktracer).MethodByName("StackTrace")

		if stacktracerStackTrace.IsValid() {
			method = stacktracerStackTrace
		}
	}

	if methodStackTrace.IsValid() {
		method = methodStackTrace
	}

	if methodStackFrames.IsValid() {
		method = methodStackFrames
	}

	return method
}

func extractPcs(method reflect.Value) []uintptr {
	var pcs []uintptr

	stacktrace := method.Call(make([]reflect.Value, 0))[0]

	if stacktrace.Kind() != reflect.Slice {
		return nil
	}

	for i := 0; i < stacktrace.Len(); i++ {
		pc := stacktrace.Index(i)

		if pc.Kind() == reflect.Uintptr {
			pcs = append(pcs, uintptr(pc.Uint()))
			continue
		}

		if pc.Kind() == reflect.Struct {
			field := pc.FieldByName("ProgramCounter")
			if field.IsValid() && field.Kind() == reflect.Uintptr {
				pcs = append(pcs, uintptr(field.Uint()))
				continue
			}
		}
	}

	return pcs
}

// extractXErrorsPC extracts program counters from error values compatible with
// the error types from golang.org/x/xerrors.
//
// It returns nil if err is not compatible with errors from that package or if
// no program counters are stored in err.
func extractXErrorsPC(err error) []uintptr {
	// This implementation uses the reflect package to avoid a hard dependency
	// on third-party packages.

	// We don't know if err matches the expected type. For simplicity, instead
	// of trying to account for all possible ways things can go wrong, some
	// assumptions are made and if they are violated the code will panic. We
	// recover from any panic and ignore it, returning nil.
	//nolint: errcheck
	defer func() { recover() }()

	field := reflect.ValueOf(err).Elem().FieldByName("frame") // type Frame struct{ frames [3]uintptr }
	field = field.FieldByName("frames")
	field = field.Slice(1, field.Len()) // drop first pc pointing to xerrors.New
	pc := make([]uintptr, field.Len())
	for i := 0; i < field.Len(); i++ {
		pc[i] = uintptr(field.Index(i).Uint())
	}
	return pc
}
