package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thaolaptrinh/spatial-server/internal/types"
)

var ErrNoSnapshot = errors.New("no snapshot")

type SnapshotStore struct {
	pool *pgxpool.Pool
}

func NewSnapshotStore(pool *pgxpool.Pool) *SnapshotStore {
	return &SnapshotStore{pool: pool}
}

func (s *SnapshotStore) Save(ctx context.Context, zoneID, runtimeID string, snapshot []byte, tick int64) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO zone_state (zone_id, runtime_id, snapshot, tick_count) VALUES ($1,$2,$3,$4)`,
		zoneID, runtimeID, snapshot, tick)
	if err != nil {
		return fmt.Errorf("save snapshot %s: %w", zoneID, err)
	}
	return nil
}

func (s *SnapshotStore) Latest(ctx context.Context, zoneID types.ZoneID) ([]byte, int64, error) {
	var snapshot []byte
	var tick int64
	err := s.pool.QueryRow(ctx,
		`SELECT snapshot, tick_count FROM zone_state WHERE zone_id=$1 ORDER BY taken_at DESC LIMIT 1`,
		string(zoneID)).Scan(&snapshot, &tick)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, fmt.Errorf("snapshot %s: %w", zoneID, ErrNoSnapshot)
		}
		return nil, 0, fmt.Errorf("latest snapshot %s: %w", zoneID, err)
	}
	return snapshot, tick, nil
}
