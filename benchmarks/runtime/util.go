package runtime

import (
	"os"
	"path/filepath"
)

// moduleRoot walks up from the working directory to find the directory
// containing go.mod, so the benchmark can write reports at a stable path
// regardless of which package directory `go test` runs from.
func moduleRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
