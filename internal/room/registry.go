package room

import (
	"fmt"
	"sync"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type ServerInfo struct {
	ID        types.ServerID
	Host      string
	Port      int
	Status    types.ServerStatus
	MaxZones  int
	ZoneCount int
	LastBeat  time.Time
}

type ServerRegistry struct {
	mu  sync.RWMutex
	svr map[types.ServerID]*ServerInfo
}

func NewServerRegistry() *ServerRegistry {
	return &ServerRegistry{svr: make(map[types.ServerID]*ServerInfo)}
}

func (r *ServerRegistry) Register(info *ServerInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.svr[info.ID]; exists {
		return fmt.Errorf("server %s: %w", info.ID, types.ErrConflict)
	}
	info.LastBeat = time.Now()
	r.svr[info.ID] = info
	return nil
}

func (r *ServerRegistry) Get(id types.ServerID) (*ServerInfo, bool) {
	r.mu.RLock()
	s, ok := r.svr[id]
	r.mu.RUnlock()
	return s, ok
}

func (r *ServerRegistry) Heartbeat(id types.ServerID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.svr[id]
	if !ok {
		return fmt.Errorf("server %s: %w", id, types.ErrNotFound)
	}
	s.LastBeat = time.Now()
	s.Status = types.ServerStatusActive
	return nil
}

func (r *ServerRegistry) Remove(id types.ServerID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.svr, id)
}

func (r *ServerRegistry) LeastLoaded() (*ServerInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best *ServerInfo
	for _, s := range r.svr {
		if s.Status != types.ServerStatusActive {
			continue
		}
		if s.ZoneCount >= s.MaxZones {
			continue
		}
		if best == nil || s.ZoneCount < best.ZoneCount {
			best = s
		}
	}
	if best == nil {
		return nil, false
	}
	return best, true
}

type ZoneOwnership struct {
	mu    sync.RWMutex
	zones map[string]types.ServerID
}

func NewZoneOwnership() *ZoneOwnership {
	return &ZoneOwnership{zones: make(map[string]types.ServerID)}
}

func (zo *ZoneOwnership) Claim(zoneID string, serverID types.ServerID) error {
	zo.mu.Lock()
	defer zo.mu.Unlock()
	if _, exists := zo.zones[zoneID]; exists {
		return fmt.Errorf("zone %s: %w", zoneID, types.ErrConflict)
	}
	zo.zones[zoneID] = serverID
	return nil
}

func (zo *ZoneOwnership) Release(zoneID string, serverID types.ServerID) error {
	zo.mu.Lock()
	defer zo.mu.Unlock()
	owner, ok := zo.zones[zoneID]
	if !ok {
		return fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
	}
	if owner != serverID {
		return fmt.Errorf("zone %s: %w", zoneID, types.ErrNotOwned)
	}
	delete(zo.zones, zoneID)
	return nil
}

func (zo *ZoneOwnership) Lookup(zoneID string) (types.ServerID, bool) {
	zo.mu.RLock()
	owner, ok := zo.zones[zoneID]
	zo.mu.RUnlock()
	return owner, ok
}

type WatcherFanout struct {
	mu       sync.Mutex
	channels map[string]chan *spatialserverv1.OwnershipChange
	nextID   int
}

func NewWatcherFanout() *WatcherFanout {
	return &WatcherFanout{channels: make(map[string]chan *spatialserverv1.OwnershipChange)}
}

func (f *WatcherFanout) Subscribe() (string, <-chan *spatialserverv1.OwnershipChange) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := fmt.Sprintf("w%d", f.nextID)
	ch := make(chan *spatialserverv1.OwnershipChange, 16)
	f.channels[id] = ch
	return id, ch
}

func (f *WatcherFanout) Unsubscribe(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ch, ok := f.channels[id]; ok {
		close(ch)
		delete(f.channels, id)
	}
}

func (f *WatcherFanout) Broadcast(change *spatialserverv1.OwnershipChange) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ch := range f.channels {
		select {
		case ch <- change:
		default:
		}
	}
}

func ResolveZone(zo *ZoneOwnership, reg *ServerRegistry, zoneID string) (types.ServerID, string, int, error) {
	serverID, ok := zo.Lookup(zoneID)
	if !ok {
		return "", "", 0, fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
	}
	info, ok := reg.Get(serverID)
	if !ok {
		return "", "", 0, fmt.Errorf("server %s: %w", serverID, types.ErrNotFound)
	}
	return serverID, info.Host, info.Port, nil
}
