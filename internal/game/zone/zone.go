package zone

import (
	"fmt"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

type GridCoord struct {
	X int
	Y int
}

type Zone struct {
	ID        types.ZoneID
	RuntimeID types.RuntimeID
	Grid      GridCoord
	Size      float64
	Status    types.ZoneStatus
	ServerID  types.ServerID
}

func New(id types.ZoneID, runtimeID types.RuntimeID, gridX, gridY int, size float64) *Zone {
	return &Zone{
		ID:        id,
		RuntimeID: runtimeID,
		Grid:      GridCoord{X: gridX, Y: gridY},
		Size:      size,
		Status:    types.ZoneStatusUnowned,
	}
}

func (z *Zone) Claim(serverID types.ServerID) error {
	if !z.Status.ValidTransition(types.ZoneStatusActive) {
		return fmt.Errorf("claim zone %s: %w (current=%s)", z.ID, types.ErrConflict, z.Status)
	}
	z.Status = types.ZoneStatusActive
	z.ServerID = serverID
	return nil
}

func (z *Zone) Release() error {
	if z.Status != types.ZoneStatusActive && z.Status != types.ZoneStatusOrphan {
		return fmt.Errorf("release zone %s: %w (current=%s)", z.ID, types.ErrInvalidArg, z.Status)
	}
	z.Status = types.ZoneStatusUnowned
	z.ServerID = ""
	return nil
}

func (z *Zone) BeginTransfer() error {
	if !z.Status.ValidTransition(types.ZoneStatusTransferring) {
		return fmt.Errorf("transfer zone %s: %w (current=%s)", z.ID, types.ErrConflict, z.Status)
	}
	z.Status = types.ZoneStatusTransferring
	return nil
}

func (z *Zone) CompleteTransfer(serverID types.ServerID) error {
	if z.Status != types.ZoneStatusTransferring {
		return fmt.Errorf("complete transfer zone %s: %w (current=%s)", z.ID, types.ErrInvalidArg, z.Status)
	}
	z.Status = types.ZoneStatusActive
	z.ServerID = serverID
	return nil
}

func (z *Zone) MarkOrphan() {
	z.Status = types.ZoneStatusOrphan
}

func (z *Zone) IsOwnedBy(serverID types.ServerID) bool {
	return z.Status == types.ZoneStatusActive && z.ServerID == serverID
}

func AdjacentZones(center GridCoord, radius int) []GridCoord {
	var coords []GridCoord
	for x := center.X - radius; x <= center.X+radius; x++ {
		for y := center.Y - radius; y <= center.Y+radius; y++ {
			coords = append(coords, GridCoord{X: x, Y: y})
		}
	}
	return coords
}
