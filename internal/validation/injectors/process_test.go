//go:build validation

package injectors

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type mockInfra struct {
	procs []*os.Process
}

func (m *mockInfra) Endpoint(id validation.ResourceID) (string, error)       { return "", nil }
func (m *mockInfra) Processes(id validation.ResourceID) ([]*os.Process, error) { return m.procs, nil }
func (m *mockInfra) Database(id validation.ResourceID) (string, error)         { return "", nil }
func (m *mockInfra) RuntimeNodes() int                                          { return len(m.procs) }
func (m *mockInfra) DialServices(ids ...validation.ResourceID) error            { return nil }
func (m *mockInfra) Close() error                                               { return nil }

func TestProcessFreeze_InjectRecover(t *testing.T) {
	cmd := exec.Command("sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep: %v", err)
	}
	defer func() { cmd.Process.Signal(syscall.SIGKILL); cmd.Wait() }()

	infra := &mockInfra{procs: []*os.Process{cmd.Process}}
	freeze := NewProcessFreeze(validation.ResourceRuntime, 100*time.Millisecond)

	if err := freeze.Inject(context.Background(), infra); err != nil {
		t.Fatalf("inject: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := freeze.Recover(context.Background(), infra); err != nil {
		t.Fatalf("recover: %v", err)
	}
}
