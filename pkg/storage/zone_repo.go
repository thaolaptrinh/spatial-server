package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

type ZoneRepository struct{ pool *pgxpool.Pool }

func NewZoneRepository(pool *pgxpool.Pool) *ZoneRepository { return &ZoneRepository{pool: pool} }

func (r *ZoneRepository) EnsureRow(ctx context.Context, zoneID, runtimeID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO zones (id, runtime_id, grid_x, grid_y, status)
		 VALUES ($1,$2,0,0,'unowned') ON CONFLICT (id) DO NOTHING`, zoneID, runtimeID)
	if err != nil {
		return fmt.Errorf("ensure zone %s: %w", zoneID, err)
	}
	return nil
}

func (r *ZoneRepository) Claim(ctx context.Context, zoneID string, runtimeID types.RuntimeID, serverID types.ServerID) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE zones SET server_id=$1, status='active' WHERE id=$2 AND server_id IS NULL`, serverID, zoneID)
	if err != nil {
		return fmt.Errorf("claim zone %s: %w", zoneID, err)
	}
	if tag.RowsAffected() == 0 {
		var owner *string
		qerr := r.pool.QueryRow(ctx, `SELECT server_id FROM zones WHERE id=$1`, zoneID).Scan(&owner)
		if errors.Is(qerr, pgx.ErrNoRows) {
			return fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
		}
		if owner != nil && *owner != "" && *owner != string(serverID) {
			return fmt.Errorf("zone %s: %w", zoneID, types.ErrConflict)
		}
	}
	return nil
}

func (r *ZoneRepository) Release(ctx context.Context, zoneID string, serverID types.ServerID) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE zones SET server_id=NULL, status='unowned' WHERE id=$1 AND server_id=$2`, zoneID, serverID)
	if err != nil {
		return fmt.Errorf("release zone %s: %w", zoneID, err)
	}
	if tag.RowsAffected() == 0 {
		var owner *string
		qerr := r.pool.QueryRow(ctx, `SELECT server_id FROM zones WHERE id=$1`, zoneID).Scan(&owner)
		if errors.Is(qerr, pgx.ErrNoRows) {
			return fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
		}
		return fmt.Errorf("zone %s: %w", zoneID, types.ErrNotOwned)
	}
	return nil
}

func (r *ZoneRepository) Lookup(ctx context.Context, zoneID string) (types.ServerID, error) {
	var owner *string
	err := r.pool.QueryRow(ctx, `SELECT server_id FROM zones WHERE id=$1`, zoneID).Scan(&owner)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
		}
		return "", fmt.Errorf("lookup zone %s: %w", zoneID, err)
	}
	if owner == nil || *owner == "" {
		return "", fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
	}
	return types.ServerID(*owner), nil
}
