package types

import (
	"testing"
)

func TestNewEntityID(t *testing.T) {
	id := NewEntityID()
	if id == "" {
		t.Fatal("NewEntityID returned empty")
	}
}

func TestNewEntityID_Unique(t *testing.T) {
	ids := make(map[EntityID]bool)
	for i := 0; i < 100; i++ {
		id := NewEntityID()
		if ids[id] {
			t.Fatalf("duplicate entity ID: %s", id)
		}
		ids[id] = true
	}
}

func TestNewRuntimeID_Unique(t *testing.T) {
	ids := make(map[RuntimeID]bool)
	for i := 0; i < 100; i++ {
		id := NewRuntimeID()
		if ids[id] {
			t.Fatalf("duplicate runtime ID: %s", id)
		}
		ids[id] = true
	}
}

func TestNewZoneID_Deterministic(t *testing.T) {
	rid := RuntimeID("r1")
	id1 := NewZoneID(rid, 0, 0)
	id2 := NewZoneID(rid, 0, 0)
	if id1 != id2 {
		t.Errorf("NewZoneID not deterministic: %q != %q", id1, id2)
	}
}

func TestNewZoneID_DifferentGrid(t *testing.T) {
	rid := RuntimeID("r1")
	id1 := NewZoneID(rid, 0, 0)
	id2 := NewZoneID(rid, 1, 0)
	if id1 == id2 {
		t.Error("NewZoneID should differ for different grid coords")
	}
}
