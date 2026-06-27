package room

import (
	"fmt"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

type OwnershipChangeListener func(zoneID string, owner types.ServerID, status types.ZoneStatus)

type MigrationCoordinator struct {
	table     *zoneTable
	listeners []OwnershipChangeListener
}

type zoneTable struct {
	rows map[string]zoneRow
}

type zoneRow struct {
	owner  types.ServerID
	status types.ZoneStatus
}

func newZoneTable() *zoneTable {
	return &zoneTable{rows: make(map[string]zoneRow)}
}

func (t *zoneTable) set(zoneID string, owner types.ServerID, status types.ZoneStatus) error {
	t.rows[zoneID] = zoneRow{owner: owner, status: status}
	return nil
}

func (t *zoneTable) get(zoneID string) (zoneRow, bool) {
	r, ok := t.rows[zoneID]
	return r, ok
}

func (t *zoneTable) update(zoneID string, owner types.ServerID, status types.ZoneStatus) {
	t.rows[zoneID] = zoneRow{owner: owner, status: status}
}

func NewMigrationCoordinator() *MigrationCoordinator {
	return &MigrationCoordinator{table: newZoneTable()}
}

func (m *MigrationCoordinator) WithListener(l OwnershipChangeListener) *MigrationCoordinator {
	m.listeners = append(m.listeners, l)
	return m
}

func (m *MigrationCoordinator) PrepareTransfer(zoneID string, target types.ServerID) (bool, error) {
	row, ok := m.table.get(zoneID)
	if !ok {
		return false, fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
	}
	if row.status != types.ZoneStatusActive {
		return false, fmt.Errorf("zone %s status %s: %w", zoneID, row.status, types.ErrConflict)
	}
	m.table.update(zoneID, row.owner, types.ZoneStatusTransferring)
	return true, nil
}

func (m *MigrationCoordinator) CompleteTransfer(zoneID string, target types.ServerID) error {
	row, ok := m.table.get(zoneID)
	if !ok {
		return fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
	}
	if row.status != types.ZoneStatusTransferring {
		return fmt.Errorf("zone %s status %s: %w", zoneID, row.status, types.ErrInvalidArg)
	}
	m.table.update(zoneID, target, types.ZoneStatusActive)
	for _, l := range m.listeners {
		l(zoneID, target, types.ZoneStatusActive)
	}
	return nil
}

func (m *MigrationCoordinator) Status(zoneID string) (types.ZoneStatus, bool) {
	row, ok := m.table.get(zoneID)
	return row.status, ok
}
