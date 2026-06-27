package room

import (
	"context"
	"log/slog"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

type SweeperConfig struct {
	Interval      time.Duration
	MissThreshold time.Duration
}

type ReassignFunc func(zoneID string, target types.ServerID)

type Sweeper struct {
	reg      *ServerRegistry
	zo       *ZoneOwnership
	cfg      SweeperConfig
	reassign ReassignFunc
}

func NewSweeper(reg *ServerRegistry, zo *ZoneOwnership, cfg SweeperConfig, reassign ReassignFunc) *Sweeper {
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.MissThreshold == 0 {
		cfg.MissThreshold = 15 * time.Second
	}
	return &Sweeper{reg: reg, zo: zo, cfg: cfg, reassign: reassign}
}

func (s *Sweeper) Run(ctx context.Context) {
	t := time.NewTicker(s.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sweep(time.Now())
		}
	}
}

func (s *Sweeper) sweep(now time.Time) {
	s.reg.mu.RLock()
	dead := make([]types.ServerID, 0)
	for id, info := range s.reg.svr {
		if info.Status == types.ServerStatusShutdown {
			continue
		}
		if now.Sub(info.LastBeat) > s.cfg.MissThreshold {
			dead = append(dead, id)
		}
	}
	s.reg.mu.RUnlock()

	for _, id := range dead {
		s.reg.mu.Lock()
		if info, ok := s.reg.svr[id]; ok {
			info.Status = types.ServerStatusShutdown
		}
		s.reg.mu.Unlock()
		s.reassignZonesOf(id)
		slog.Warn("server marked shutdown after heartbeat timeout", slog.String("server_id", string(id)))
	}
}

func (s *Sweeper) reassignZonesOf(dead types.ServerID) {
	s.zo.mu.Lock()
	orphaned := make([]string, 0)
	for zoneID, owner := range s.zo.zones {
		if owner == dead {
			orphaned = append(orphaned, zoneID)
			delete(s.zo.zones, zoneID)
		}
	}
	s.zo.mu.Unlock()

	for _, zoneID := range orphaned {
		target, ok := s.reg.LeastLoaded()
		if !ok {
			slog.Error("no active server to receive orphan zone", slog.String("zone_id", zoneID))
			continue
		}
		if s.reassign != nil {
			s.reassign(zoneID, target.ID)
		}
	}
}
