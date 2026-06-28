package room

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/internal/room"
)

type ServerRepository struct{ pool *pgxpool.Pool }

func NewServerRepository(pool *pgxpool.Pool) *ServerRepository { return &ServerRepository{pool: pool} }

func (r *ServerRepository) Register(ctx context.Context, info *room.NodeDescriptor) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO game_servers (id, host, port, status, max_zones, registered_at, last_heartbeat)
		 VALUES ($1,$2,$3,$4,$5,NOW(),NOW())
		 ON CONFLICT (id) DO UPDATE SET host=EXCLUDED.host, port=EXCLUDED.port, max_zones=EXCLUDED.max_zones`,
		info.NodeID, info.Host, info.Port, info.Status.String(), info.Capacity.MaxZones)
	if err != nil {
		return fmt.Errorf("register server %s: %w", info.NodeID, err)
	}
	return nil
}

func (r *ServerRepository) Heartbeat(ctx context.Context, id types.ServerID, _ room.NodeLoad) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE game_servers SET status='active', last_heartbeat=NOW() WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("heartbeat server %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("server %s: %w", id, types.ErrNotFound)
	}
	return nil
}

func (r *ServerRepository) Get(ctx context.Context, id types.ServerID) (*room.NodeDescriptor, error) {
	row := r.pool.QueryRow(ctx, `SELECT id,host,port,status,max_zones FROM game_servers WHERE id=$1`, id)
	return scanServer(row)
}

func (r *ServerRepository) List(ctx context.Context) ([]*room.NodeDescriptor, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id,host,port,status,max_zones FROM game_servers WHERE status='active'`)
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	defer rows.Close()
	var out []*room.NodeDescriptor
	for rows.Next() {
		var info room.NodeDescriptor
		var statusStr string
		if err := rows.Scan(&info.NodeID, &info.Host, &info.Port, &statusStr, &info.Capacity.MaxZones); err != nil {
			return nil, err
		}
		info.Status = types.ServerStatusActive
		out = append(out, &info)
	}
	return out, nil
}

func (r *ServerRepository) Remove(ctx context.Context, id types.ServerID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM game_servers WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("remove server %s: %w", id, err)
	}
	return nil
}

func scanServer(row pgx.Row) (*room.NodeDescriptor, error) {
	var info room.NodeDescriptor
	var statusStr string
	if err := row.Scan(&info.NodeID, &info.Host, &info.Port, &statusStr, &info.Capacity.MaxZones); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("server: %w", types.ErrNotFound)
		}
		return nil, err
	}
	info.Status = types.ServerStatusActive
	info.LastHeartbeat = time.Now()
	return &info, nil
}
