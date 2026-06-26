package entity

import (
	"testing"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

func TestNewEntity(t *testing.T) {
	id := types.EntityID("test-entity-1")
	rid := types.RuntimeID("runtime-1")
	e := New(id, "avatar", rid)

	if e.ID != id {
		t.Errorf("Entity.ID = %q, want %q", e.ID, id)
	}
	if e.Type != "avatar" {
		t.Errorf("Entity.Type = %q, want %q", e.Type, "avatar")
	}
	if e.RuntimeID != rid {
		t.Errorf("Entity.RuntimeID = %q, want %q", e.RuntimeID, rid)
	}
	if e.Attrs == nil {
		t.Error("Entity.Attrs should not be nil")
	}
}

func TestEntityDefaultPosition(t *testing.T) {
	e := New(types.EntityID("e1"), "npc", types.RuntimeID("r1"))
	if e.Position.X != 0 || e.Position.Y != 0 || e.Position.Z != 0 {
		t.Error("new entity should have zero position")
	}
}

func TestEntityAttributes(t *testing.T) {
	e := New(types.EntityID("e1"), "item", types.RuntimeID("r1"))
	e.Attrs["health"] = []byte("100")
	e.Attrs["name"] = []byte("sword")

	if string(e.Attrs["health"]) != "100" {
		t.Errorf("health = %q, want %q", e.Attrs["health"], "100")
	}
	if string(e.Attrs["name"]) != "sword" {
		t.Errorf("name = %q, want %q", e.Attrs["name"], "sword")
	}
}
