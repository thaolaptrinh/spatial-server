//go:build validation

package recovery

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type nilInfra struct{}

func (n *nilInfra) Endpoint(id validation.ResourceID) (string, error)       { return "", nil }
func (n *nilInfra) Processes(id validation.ResourceID) ([]*os.Process, error) { return nil, nil }
func (n *nilInfra) Database(id validation.ResourceID) (string, error)         { return "", nil }
func (n *nilInfra) RuntimeNodes() int                                          { return 0 }
func (n *nilInfra) DialServices(ids ...validation.ResourceID) error            { return nil }
func (n *nilInfra) Close() error                                               { return nil }

func TestCompositeWaiter_AllOfMet(t *testing.T) {
	w := NewComposite(ModeAll, FixedDelay("a", time.Millisecond), FixedDelay("b", time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := w.Wait(ctx, &nilInfra{}); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestCompositeWaiter_AnyOfMet(t *testing.T) {
	w := NewComposite(ModeAny, FixedDelay("a", time.Millisecond), FixedDelay("b", 10*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := w.Wait(ctx, &nilInfra{}); err != nil {
		t.Fatalf("expected any-of success, got %v", err)
	}
}

func TestCompositeWaiter_Timeout(t *testing.T) {
	w := NewComposite(ModeAll, FixedDelay("a", time.Second), FixedDelay("b", time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := w.Wait(ctx, &nilInfra{}); err == nil {
		t.Fatal("expected timeout")
	}
}

func TestCompositeWaiter_Empty(t *testing.T) {
	ctx := context.Background()
	if err := NewComposite(ModeAll).Wait(ctx, &nilInfra{}); err != nil {
		t.Fatalf("empty conditions should succeed, got %v", err)
	}
}
