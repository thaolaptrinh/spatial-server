//go:build validation

package game

import "github.com/thaolaptrinh/spatial-server/internal/types"

type Snapshot struct {
	EntityCount int
	GhostCount  int
	ZoneOwners  map[types.ZoneID]types.ServerID
	QueueDepths map[string]int
}

func (g *Game) Snapshot() Snapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()
	s := Snapshot{
		EntityCount: len(g.Entities),
		GhostCount:  len(g.ghosts),
		ZoneOwners:  make(map[types.ZoneID]types.ServerID, len(g.Zones)),
		QueueDepths: make(map[string]int, 3),
	}
	for zid := range g.Zones {
		s.ZoneOwners[zid] = g.ServerID
	}
	s.QueueDepths["inbox"] = len(g.Inbox)
	s.QueueDepths["events"] = len(g.Events)
	s.QueueDepths["cmds"] = len(g.cmds)
	return s
}
