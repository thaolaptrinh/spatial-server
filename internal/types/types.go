package types

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrInvalidArg = errors.New("invalid argument")
	ErrNotOwned   = errors.New("not owned")
	ErrNotEmpty   = errors.New("not empty")
)

type EntityID string

type ZoneID string

func (z ZoneID) String() string { return string(z) }

type RuntimeID string

type ServerID string

type Vector3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type Rotation struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
	W float64 `json:"w"`
}

type RuntimeStatus int

const (
	RuntimeStatusCreating  RuntimeStatus = 0
	RuntimeStatusActive    RuntimeStatus = 1
	RuntimeStatusDraining  RuntimeStatus = 2
	RuntimeStatusDestroyed RuntimeStatus = 3
)

func (s RuntimeStatus) String() string {
	switch s {
	case RuntimeStatusCreating:
		return "creating"
	case RuntimeStatusActive:
		return "active"
	case RuntimeStatusDraining:
		return "draining"
	case RuntimeStatusDestroyed:
		return "destroyed"
	default:
		return fmt.Sprintf("RuntimeStatus(%d)", s)
	}
}

type ZoneStatus int

const (
	ZoneStatusUnowned      ZoneStatus = 0
	ZoneStatusActive       ZoneStatus = 1
	ZoneStatusTransferring ZoneStatus = 2
	ZoneStatusOrphan       ZoneStatus = 3
)

func (s ZoneStatus) String() string {
	switch s {
	case ZoneStatusUnowned:
		return "unowned"
	case ZoneStatusActive:
		return "active"
	case ZoneStatusTransferring:
		return "transferring"
	case ZoneStatusOrphan:
		return "orphan"
	default:
		return fmt.Sprintf("ZoneStatus(%d)", s)
	}
}

func (s ZoneStatus) Valid() bool {
	return s >= ZoneStatusUnowned && s <= ZoneStatusOrphan
}

func (s ZoneStatus) ValidTransition(to ZoneStatus) bool {
	transitions := map[ZoneStatus][]ZoneStatus{
		ZoneStatusUnowned:      {ZoneStatusActive},
		ZoneStatusActive:       {ZoneStatusTransferring, ZoneStatusUnowned},
		ZoneStatusTransferring: {ZoneStatusActive, ZoneStatusOrphan},
		ZoneStatusOrphan:       {ZoneStatusActive, ZoneStatusUnowned},
	}
	next, ok := transitions[s]
	if !ok {
		return false
	}
	for _, n := range next {
		if n == to {
			return true
		}
	}
	return false
}

type ServerStatus int

const (
	ServerStatusJoining  ServerStatus = 0
	ServerStatusActive   ServerStatus = 1
	ServerStatusDraining ServerStatus = 2
	ServerStatusShutdown ServerStatus = 3
)

func (s ServerStatus) String() string {
	switch s {
	case ServerStatusJoining:
		return "joining"
	case ServerStatusActive:
		return "active"
	case ServerStatusDraining:
		return "draining"
	case ServerStatusShutdown:
		return "shutdown"
	default:
		return fmt.Sprintf("ServerStatus(%d)", s)
	}
}
