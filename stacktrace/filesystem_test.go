package stacktrace_test

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yext/glog-contrib/stacktrace"
)

var gopath string
var gopathFolderName string

func init() {
	gopath = os.Getenv("GOPATH")
	gopathFolderName = filepath.Base(gopath)
}

func TestGopathRelativeFile(t *testing.T) {
	assert.Equal(t, "yext/examples/example.go", stacktrace.GopathRelativeFile(path.Join(gopathFolderName, "src/yext/examples/example.go")))
	assert.Equal(t, "folder/that-is-not-src/examples/example.go", stacktrace.GopathRelativeFile("folder/that-is-not-src/examples/example.go"))
	assert.Equal(t, "/path/to/folder/that-is-not-src/examples/example.go", stacktrace.GopathRelativeFile("/path/to/folder/that-is-not-src/examples/example.go"))
}

func TestGuessAbsPath(t *testing.T) {
	assert.Equal(t, path.Join(gopath, "src/yext/example.go"), stacktrace.GuessAbsPath(path.Join(gopathFolderName, "src/yext/example.go")))
	assert.Equal(t, "bazel-out/darwin-fastbuild/bin/src/example.go", stacktrace.GuessAbsPath("bazel-out/darwin-fastbuild/bin/src/example.go"))
	assert.Equal(t, "external/com_github_grpc_ecosystem_go_grpc_middleware/example.go", stacktrace.GuessAbsPath("external/com_github_grpc_ecosystem_go_grpc_middleware/example.go"))
	assert.Equal(t, "GOROOT/src/runtime/asm_amd64.s", stacktrace.GuessAbsPath("GOROOT/src/runtime/asm_amd64.s"))
	assert.Equal(t, "/path/to/foo/bar.go", stacktrace.GuessAbsPath("/path/to/foo/bar.go"))
	assert.Equal(t, path.Join(gopath, "foo/bar.go"), stacktrace.GuessAbsPath(path.Join(gopath, "foo/bar.go")))
}
