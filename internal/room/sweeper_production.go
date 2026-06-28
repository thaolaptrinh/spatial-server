package room

import (
	"context"
	"log/slog"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type (
	ServerDeadLister interface {
		ListDead(ctx context.Context, since time.Duration) ([]types.ServerID, error)
		MarkShutdown(ctx context.Context, id types.ServerID) error
	}

	ZoneByOwnerLister interface {
		ListByServer(ctx context.Context, serverID types.ServerID) ([]string, error)
	}
)

type ProductionSweeper struct {
	servers   ServerStore
	zones     ZoneStore
	allocator Allocator
	fanout    *WatcherFanout
	cfg       SweeperConfig
}

func NewProductionSweeper(servers ServerStore, zones ZoneStore, allocator Allocator, fanout *WatcherFanout, cfg SweeperConfig) *ProductionSweeper {
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.MissThreshold == 0 {
		cfg.MissThreshold = 15 * time.Second
	}
	return &ProductionSweeper{servers: servers, zones: zones, allocator: allocator, fanout: fanout, cfg: cfg}
}

func (s *ProductionSweeper) Run(ctx context.Context) {
	t := time.NewTicker(s.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sweep(ctx, time.Now())
		}
	}
}

func (s *ProductionSweeper) sweep(ctx context.Context, now time.Time) {
	deadLister, ok := s.servers.(ServerDeadLister)
	if !ok {
		slog.Warn("production sweeper: ServerStore does not implement ServerDeadLister")
		return
	}
	dead, err := deadLister.ListDead(ctx, s.cfg.MissThreshold)
	if err != nil {
		slog.Warn("production sweeper: list dead servers", "error", err.Error())
		return
	}

	for _, deadID := range dead {
		if shutdownMarker, ok := s.servers.(interface {
			MarkShutdown(context.Context, types.ServerID) error
		}); ok {
			if err := shutdownMarker.MarkShutdown(ctx, deadID); err != nil {
				slog.Warn("production sweeper: mark shutdown", "server_id", string(deadID), "error", err.Error())
				continue
			}
		}

		slog.Warn("server marked shutdown after heartbeat timeout", "server_id", string(deadID))
		s.reassignZonesFor(ctx, deadID)
	}
}

func (s *ProductionSweeper) reassignZonesFor(ctx context.Context, dead types.ServerID) {
	byOwnerLister, ok := s.zones.(ZoneByOwnerLister)
	if !ok {
		slog.Warn("production sweeper: ZoneStore does not implement ZoneByOwnerLister")
		return
	}
	zoneIDs, err := byOwnerLister.ListByServer(ctx, dead)
	if err != nil {
		slog.Warn("production sweeper: list zones by server", "server_id", string(dead), "error", err.Error())
		return
	}

	for _, zoneID := range zoneIDs {
		if err := s.zones.Release(ctx, zoneID, dead); err != nil {
			slog.Warn("production sweeper: release orphan zone", "zone_id", zoneID, "error", err.Error())
			continue
		}
		nodes, err := s.servers.List(ctx)
		if err != nil || len(nodes) == 0 {
			slog.Error("production sweeper: no active server to receive orphan zone", "zone_id", zoneID)
			continue
		}
		target, err := s.allocator.Select(nodes)
		if err != nil {
			slog.Error("production sweeper: allocator returned no target", "zone_id", zoneID, "error", err.Error())
			continue
		}
		if err := s.zones.Claim(ctx, zoneID, "", target.NodeID); err != nil {
			slog.Error("production sweeper: claim reassigned zone", "zone_id", zoneID, "target", string(target.NodeID), "error", err.Error())
			continue
		}
		if s.fanout != nil {
			s.fanout.Broadcast(&spatialserverv1.OwnershipChange{
				ZoneId:   zoneID,
				ServerId: string(target.NodeID),
				Host:     target.Host,
				Port:     int32(target.Port),
			})
		}
		slog.Info("orphan zone reassigned", "zone_id", zoneID, "new_owner", string(target.NodeID))
	}
}
