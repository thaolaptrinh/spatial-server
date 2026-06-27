package zone

import (
	"testing"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

func TestNewZone(t *testing.T) {
	id := types.ZoneID("zone-1")
	rid := types.RuntimeID("runtime-1")
	z := New(id, rid, 0, 0, 100)

	if z.ID != id {
		t.Errorf("Zone.ID = %q, want %q", z.ID, id)
	}
	if z.RuntimeID != rid {
		t.Errorf("Zone.RuntimeID = %q, want %q", z.RuntimeID, rid)
	}
	if z.Status != types.ZoneStatusUnowned {
		t.Errorf("Zone.Status = %s, want %s", z.Status, types.ZoneStatusUnowned)
	}
	if z.Size != 100 {
		t.Errorf("Zone.Size = %f, want %f", z.Size, 100.0)
	}
}

func TestZoneClaimRelease(t *testing.T) {
	z := New(types.ZoneID("z1"), types.RuntimeID("r1"), 1, 1, 100)
	sid := types.ServerID("gs-1")

	if err := z.Claim(sid); err != nil {
		t.Fatalf("Claim failed: %v", err)
	}
	if z.Status != types.ZoneStatusActive {
		t.Errorf("Status = %s, want %s", z.Status, types.ZoneStatusActive)
	}
	if z.ServerID != sid {
		t.Errorf("ServerID = %q, want %q", z.ServerID, sid)
	}
	if !z.IsOwnedBy(sid) {
		t.Error("IsOwnedBy should be true")
	}

	if err := z.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}
	if z.Status != types.ZoneStatusUnowned {
		t.Errorf("Status = %s, want %s", z.Status, types.ZoneStatusUnowned)
	}
}

func TestZoneDoubleClaimFails(t *testing.T) {
	z := New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)
	if err := z.Claim(types.ServerID("gs-1")); err != nil {
		t.Fatalf("first claim failed: %v", err)
	}
	if err := z.Claim(types.ServerID("gs-2")); err == nil {
		t.Error("second claim should fail")
	}
}

func TestZoneTransfer(t *testing.T) {
	z := New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)
	if err := z.Claim(types.ServerID("gs-1")); err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if err := z.BeginTransfer(); err != nil {
		t.Fatalf("BeginTransfer failed: %v", err)
	}
	if z.Status != types.ZoneStatusTransferring {
		t.Errorf("Status = %s, want %s", z.Status, types.ZoneStatusTransferring)
	}
	if err := z.CompleteTransfer(types.ServerID("gs-2")); err != nil {
		t.Fatalf("CompleteTransfer failed: %v", err)
	}
	if z.Status != types.ZoneStatusActive {
		t.Errorf("Status = %s, want %s", z.Status, types.ZoneStatusActive)
	}
	if z.ServerID != "gs-2" {
		t.Errorf("ServerID = %q, want %q", z.ServerID, "gs-2")
	}
}

func TestZoneOrphan(t *testing.T) {
	z := New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)
	z.MarkOrphan()
	if z.Status != types.ZoneStatusOrphan {
		t.Errorf("Status = %s, want %s", z.Status, types.ZoneStatusOrphan)
	}
}

func TestAdjacentZones(t *testing.T) {
	coords := AdjacentZones(GridCoord{X: 5, Y: 5}, 1)
	if len(coords) != 9 {
		t.Errorf("AdjacentZones count = %d, want 9 (3x3 grid)", len(coords))
	}
	seen := make(map[GridCoord]bool)
	for _, c := range coords {
		if seen[c] {
			t.Errorf("duplicate coordinate %v", c)
		}
		seen[c] = true
	}
}

func TestAdjacentZonesRadius2(t *testing.T) {
	coords := AdjacentZones(GridCoord{X: 0, Y: 0}, 2)
	if len(coords) != 25 {
		t.Errorf("AdjacentZones count = %d, want 25 (5x5 grid)", len(coords))
	}
}
