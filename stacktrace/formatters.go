package stacktrace

import (
	"fmt"

	"github.com/getsentry/sentry-go"
)

// SourceFromStack retrieves the function and line where the
// event was logged from in the format "file.Function:118".
func SourceFromStack(s *sentry.Stacktrace) string {
	if s == nil || len(s.Frames) == 0 {
		return ""
	}

	f := s.Frames[len(s.Frames)-1]
	return fmt.Sprintf("%s:%d", f.Function, f.Lineno)
}
