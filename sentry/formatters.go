package sentry

import (
	"strings"

	"github.com/getsentry/sentry-go"
	"golang.org/x/xerrors"
)

// headline returns a good Headline for this error.
// Ideally, it returns a succinct summary that best conveys the error.
// Most likely, that's something close to the root cause, but that may
// be something boring like "context canceled".
func headline(err error) string {
	// Heuristic: return the error message from the second innermost error.
	// This provides context on the error, since returned errors are often constants.
	var prev error
	for {
		wrapper, ok := err.(xerrors.Wrapper)
		if !ok {
			break
		}
		prev = err
		err = wrapper.Unwrap()
	}
	if prev != nil {
		return prev.Error()
	}
	return err.Error()
}


// removeGlogPrefixFromMessage removes the glog date/level from the
// raw byte string returned from glogEvent.Message
func removeGlogPrefixFromMessage(msg []byte) string {
	message := string(msg)
	if square := strings.Index(message, "] "); square != -1 {
		message = message[square+2:]
	}

	return message
}

// cleanupMessage cleans up a message displayed as the top-line
// Sentry error by splitting at the first newline.
func cleanupMessage(msg string) string {
	return strings.Split(strings.TrimSpace(msg), "\n")[0]
}

// prependMessage prepends the given possiblePrefix to an
// existing fullMsg. If fullMsg starts with possiblePrefix
// then the prefix is removed. Otherwise the possiblePrefix
// is shown before the given message.
func prependMessage(possiblePrefix, fullMsg string) string {
	trimmedMsg := strings.TrimPrefix(fullMsg, possiblePrefix)
	trimmedMsg = strings.TrimSpace(trimmedMsg)
	trimmedMsg = strings.TrimPrefix(trimmedMsg, ":")
	if len(trimmedMsg) > 0 {
		return possiblePrefix + "\n" + trimmedMsg
	} else {
		return possiblePrefix
	}
}

// buildLevel converts a glog level to a sentry level.
// input level is one of: INFO, WARNING, ERROR or FATAL
func buildLevel(severity string) sentry.Level {
	return sentry.Level(strings.ToLower(severity))
}
