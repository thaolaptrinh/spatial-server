//go:build validation

package game

import "testing"

func TestSnapshot_InitialState(t *testing.T) {
	g := New("gs-test")
	s := g.Snapshot()
	if s.EntityCount != 0 {
		t.Fatalf("expected 0 entities, got %d", s.EntityCount)
	}
	if s.GhostCount != 0 {
		t.Fatalf("expected 0 ghosts, got %d", s.GhostCount)
	}
	if s.QueueDepths["inbox"] != 0 {
		t.Fatalf("expected 0 inbox depth, got %d", s.QueueDepths["inbox"])
	}
}
