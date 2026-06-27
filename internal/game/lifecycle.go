package game

import (
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

type SessionStatus int

const (
	SessionActive       SessionStatus = iota
	SessionDisconnected
	SessionDespawned
)

type sessionState struct {
	status         SessionStatus
	disconnectedAt time.Time
}

func (g *Game) MarkDisconnected(id types.EntityID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.Entities[id]; !ok {
		return
	}
	st := g.sessionStates[id]
	if st == nil {
		st = &sessionState{}
		g.sessionStates[id] = st
	}
	st.status = SessionDisconnected
	st.disconnectedAt = g.lifecycleClock()
}

func (g *Game) MarkReconnected(id types.EntityID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.sessionStates[id]
	if st == nil {
		st = &sessionState{}
		g.sessionStates[id] = st
	}
	st.status = SessionActive
	st.disconnectedAt = time.Time{}
}

func (g *Game) IsDisconnected(id types.EntityID) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	st := g.sessionStates[id]
	return st != nil && st.status == SessionDisconnected
}

func (g *Game) SweepDisconnected() {
	now := g.lifecycleClock()
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, st := range g.sessionStates {
		if st.status != SessionDisconnected {
			continue
		}
		if now.Sub(st.disconnectedAt) > g.reconnectWindow {
			zid, ok := g.entityZone[id]
			if ok {
				if grid, exists := g.aoiIndex[zid]; exists {
					grid.Leave(id)
				}
			}
			delete(g.Entities, id)
			delete(g.entityAOI, id)
			delete(g.entityZone, id)
			delete(g.sessionStates, id)
		}
	}
}
