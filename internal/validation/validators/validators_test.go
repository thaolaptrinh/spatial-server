package validators

import (
	"testing"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

func TestEntity_Pass(t *testing.T) {
	v := &Entity{}
	base := []validation.Evidence{{Key: validation.EvKeyEntityCount, Value: "10", Timestamp: time.Now()}}
	post := []validation.Evidence{{Key: validation.EvKeyEntityCount, Value: "10", Timestamp: time.Now()}}
	r := v.Validate(base, post)
	if r.Status != validation.StatusPass {
		t.Fatalf("expected pass, got %s: %s", r.Status, r.Detail)
	}
}

func TestEntity_Fail(t *testing.T) {
	v := &Entity{}
	r := v.Validate(
		[]validation.Evidence{{Key: validation.EvKeyEntityCount, Value: "10"}},
		[]validation.Evidence{{Key: validation.EvKeyEntityCount, Value: "5"}},
	)
	if r.Status != validation.StatusFail {
		t.Fatalf("expected fail, got %s", r.Status)
	}
}

func TestAOI_GhostLeak(t *testing.T) {
	v := &AOI{}
	r := v.Validate(
		[]validation.Evidence{{Key: validation.EvKeyGhostCount, Value: "2"}, {Key: validation.EvKeyEntityCount, Value: "10"}},
		[]validation.Evidence{{Key: validation.EvKeyGhostCount, Value: "15"}, {Key: validation.EvKeyEntityCount, Value: "10"}},
	)
	if r.Status != validation.StatusFail {
		t.Fatalf("expected ghost leak fail, got %s", r.Status)
	}
}

func TestAOI_GhostBounded(t *testing.T) {
	v := &AOI{}
	r := v.Validate(
		[]validation.Evidence{{Key: validation.EvKeyGhostCount, Value: "5"}, {Key: validation.EvKeyEntityCount, Value: "10"}},
		[]validation.Evidence{{Key: validation.EvKeyGhostCount, Value: "7"}, {Key: validation.EvKeyEntityCount, Value: "10"}},
	)
	if r.Status != validation.StatusPass {
		t.Fatalf("expected pass, got %s: %s", r.Status, r.Detail)
	}
}

func TestOwnership_Pass(t *testing.T) {
	v := &Ownership{}
	r := v.Validate(
		[]validation.Evidence{{Key: validation.EvKeyZoneOwnerCount, Value: "2"}},
		[]validation.Evidence{{Key: validation.EvKeyZoneOwnerCount, Value: "2"}},
	)
	if r.Status != validation.StatusPass {
		t.Fatalf("expected pass, got %s", r.Status)
	}
}
