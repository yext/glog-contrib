package stacktrace

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// GopathRelativeFile sanitizes the path to remove GOPATH and obtain the import path.
// Concretely, this takes the path after the last instance of '/src/'.
// This may omit some of the path if there is an src directory in a package import path.
// If there are no /src/ directories in the path, the base filename is returned.
func GopathRelativeFile(absPath string) string {
	candidates := strings.SplitAfter(absPath, "/src/")
	if len(candidates) > 0 {
		return candidates[len(candidates)-1]
	}
	return filepath.Base(absPath)
}

// GuessAbsPath guesses the proper absolute path if it is not
// provided, making a best-effort guess so that Sentry can attempt
// to augment the error with source code
func GuessAbsPath(f string) string {
	gopath := os.Getenv("GOPATH")
	// Break out if the GOPATH can't be identified, or the path
	// is already an absolute path.
	if gopath == "" || strings.HasPrefix(f, "/") {
		return f
	}

	ignoredPrefixes := []string{gopath, "external/", "GOROOT/", "bazel-"}
	for _, prefix := range ignoredPrefixes {
		if strings.HasPrefix(f, prefix) {
			return f
		}
	}

	if strings.HasPrefix(f, filepath.Base(gopath)) {
		return path.Join(filepath.Dir(gopath), f)
	} else {
		return path.Join(gopath, f)
	}
}
