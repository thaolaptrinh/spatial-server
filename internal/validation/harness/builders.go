//go:build validation

package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func moduleRoot() string {
	dir, _ := os.Getwd()
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

func buildService(t *testing.T, name string) string {
	t.Helper()
	root := moduleRoot()
	require.NotEmpty(t, root)
	binPath := filepath.Join(os.TempDir(), fmt.Sprintf("spatial-%s-%d", name, time.Now().UnixNano()))
	cmd := exec.Command("go", "build", "-tags=validation", "-o", binPath, fmt.Sprintf("./apps/%s/", name))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build %s failed:\n%s", name, string(out))
	return binPath
}
