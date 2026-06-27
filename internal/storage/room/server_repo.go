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

func (r *ServerRepository) Register(ctx context.Context, info *room.ServerInfo) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO game_servers (id, host, port, status, max_zones, registered_at, last_heartbeat)
		 VALUES ($1,$2,$3,$4,$5,NOW(),NOW())
		 ON CONFLICT (id) DO UPDATE SET host=EXCLUDED.host, port=EXCLUDED.port, max_zones=EXCLUDED.max_zones`,
		info.ID, info.Host, info.Port, info.Status.String(), info.MaxZones)
	if err != nil {
		return fmt.Errorf("register server %s: %w", info.ID, err)
	}
	return nil
}

func (r *ServerRepository) Heartbeat(ctx context.Context, id types.ServerID) error {
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

func (r *ServerRepository) Get(ctx context.Context, id types.ServerID) (*room.ServerInfo, error) {
	row := r.pool.QueryRow(ctx, `SELECT id,host,port,status,max_zones FROM game_servers WHERE id=$1`, id)
	return scanServer(row)
}

func (r *ServerRepository) LeastLoaded(ctx context.Context) (*room.ServerInfo, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id,host,port,status,max_zones FROM game_servers WHERE status='active'
		 ORDER BY (SELECT COUNT(*) FROM zones WHERE zones.server_id=game_servers.id) ASC LIMIT 1`)
	return scanServer(row)
}

func (r *ServerRepository) Remove(ctx context.Context, id types.ServerID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM game_servers WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("remove server %s: %w", id, err)
	}
	return nil
}

func scanServer(row pgx.Row) (*room.ServerInfo, error) {
	var info room.ServerInfo
	var statusStr string
	if err := row.Scan(&info.ID, &info.Host, &info.Port, &statusStr, &info.MaxZones); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("server: %w", types.ErrNotFound)
		}
		return nil, err
	}
	info.Status = types.ServerStatusActive
	info.LastBeat = time.Now()
	return &info, nil
}
