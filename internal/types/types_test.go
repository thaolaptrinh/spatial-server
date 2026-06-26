package types

import (
	"testing"
)

func TestZoneStatus_Valid(t *testing.T) {
	tests := []struct {
		name   string
		status ZoneStatus
		want   bool
	}{
		{"unowned", ZoneStatusUnowned, true},
		{"active", ZoneStatusActive, true},
		{"transferring", ZoneStatusTransferring, true},
		{"orphan", ZoneStatusOrphan, true},
		{"invalid negative", ZoneStatus(-1), false},
		{"invalid large", ZoneStatus(99), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.Valid(); got != tt.want {
				t.Errorf("ZoneStatus.Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestZoneStatus_String(t *testing.T) {
	tests := []struct {
		status ZoneStatus
		want   string
	}{
		{ZoneStatusUnowned, "unowned"},
		{ZoneStatusActive, "active"},
		{ZoneStatusTransferring, "transferring"},
		{ZoneStatusOrphan, "orphan"},
		{ZoneStatus(99), "ZoneStatus(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("ZoneStatus.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZoneStatus_ValidTransition(t *testing.T) {
	tests := []struct {
		name string
		from ZoneStatus
		to   ZoneStatus
		want bool
	}{
		{"unowned -> active", ZoneStatusUnowned, ZoneStatusActive, true},
		{"unowned -> transferring", ZoneStatusUnowned, ZoneStatusTransferring, false},
		{"active -> transferring", ZoneStatusActive, ZoneStatusTransferring, true},
		{"active -> unowned", ZoneStatusActive, ZoneStatusUnowned, true},
		{"active -> orphan", ZoneStatusActive, ZoneStatusOrphan, false},
		{"transferring -> active", ZoneStatusTransferring, ZoneStatusActive, true},
		{"transferring -> orphan", ZoneStatusTransferring, ZoneStatusOrphan, true},
		{"orphan -> active", ZoneStatusOrphan, ZoneStatusActive, true},
		{"orphan -> unowned", ZoneStatusOrphan, ZoneStatusUnowned, true},
		{"orphan -> transferring", ZoneStatusOrphan, ZoneStatusTransferring, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.from.ValidTransition(tt.to); got != tt.want {
				t.Errorf("ValidTransition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRuntimeStatus_String(t *testing.T) {
	tests := []struct {
		status RuntimeStatus
		want   string
	}{
		{RuntimeStatusCreating, "creating"},
		{RuntimeStatusActive, "active"},
		{RuntimeStatusDraining, "draining"},
		{RuntimeStatusDestroyed, "destroyed"},
		{RuntimeStatus(99), "RuntimeStatus(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("RuntimeStatus.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServerStatus_String(t *testing.T) {
	tests := []struct {
		status ServerStatus
		want   string
	}{
		{ServerStatusJoining, "joining"},
		{ServerStatusActive, "active"},
		{ServerStatusDraining, "draining"},
		{ServerStatusShutdown, "shutdown"},
		{ServerStatus(99), "ServerStatus(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("ServerStatus.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSentinelErrorsAreDistinct(t *testing.T) {
	if ErrNotFound == ErrConflict {
		t.Error("ErrNotFound and ErrConflict must be distinct")
	}
	if ErrNotFound == ErrInvalidArg {
		t.Error("ErrNotFound and ErrInvalidArg must be distinct")
	}
	if ErrNotFound == ErrNotOwned {
		t.Error("ErrNotFound and ErrNotOwned must be distinct")
	}
	if ErrNotFound == ErrNotEmpty {
		t.Error("ErrNotFound and ErrNotEmpty must be distinct")
	}
}
