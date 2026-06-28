package room

import (
	"fmt"
	"sync"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type ServerRegistry struct {
	mu  sync.RWMutex
	svr map[types.ServerID]*NodeDescriptor
}

func NewServerRegistry() *ServerRegistry {
	return &ServerRegistry{svr: make(map[types.ServerID]*NodeDescriptor)}
}

// Register records a Runtime Node described by its NodeDescriptor. Identity is
// the NodeID; re-registering an existing NodeID is a conflict. The node becomes
// active after its first heartbeat (not on registration), so a node that
// registers but never heartbeats is never a scheduling candidate.
func (r *ServerRegistry) Register(info *NodeDescriptor) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.svr[info.NodeID]; exists {
		return fmt.Errorf("node %s: %w", info.NodeID, types.ErrConflict)
	}
	info.LastHeartbeat = time.Now()
	if info.StartTime.IsZero() {
		info.StartTime = time.Now()
	}
	r.svr[info.NodeID] = info
	return nil
}

func (r *ServerRegistry) Get(id types.ServerID) (*NodeDescriptor, bool) {
	r.mu.RLock()
	s, ok := r.svr[id]
	r.mu.RUnlock()
	return s, ok
}

// Heartbeat records liveness for a node.
func (r *ServerRegistry) Heartbeat(id types.ServerID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.svr[id]
	if !ok {
		return fmt.Errorf("node %s: %w", id, types.ErrNotFound)
	}
	s.LastHeartbeat = time.Now()
	s.Status = types.ServerStatusActive
	return nil
}

// UpdateLoad records scheduling-relevant load reported by a node heartbeat.
func (r *ServerRegistry) UpdateLoad(id types.ServerID, load NodeLoad) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.svr[id]
	if !ok {
		return fmt.Errorf("node %s: %w", id, types.ErrNotFound)
	}
	s.Load = load
	s.LastHeartbeat = time.Now()
	s.Status = types.ServerStatusActive
	return nil
}

func (r *ServerRegistry) Remove(id types.ServerID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.svr, id)
}

// List returns all registered node descriptors (allocator candidates).
func (r *ServerRegistry) List() []*NodeDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*NodeDescriptor, 0, len(r.svr))
	for _, n := range r.svr {
		out = append(out, n)
	}
	return out
}

// allocator is the default scheduling policy used by LeastLoaded.
var defaultAllocator Allocator = LeastLoadedAllocator{}

// LeastLoaded selects a node using the default allocator. Returns the node and
// whether one was found.
func (r *ServerRegistry) LeastLoaded() (*NodeDescriptor, bool) {
	n, err := defaultAllocator.Select(r.List())
	if err != nil {
		return nil, false
	}
	return n, true
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

// ResolveZone returns the server ID and routable address of a zone's owner.
func ResolveZone(zo *ZoneOwnership, reg *ServerRegistry, zoneID string) (types.ServerID, string, error) {
	serverID, ok := zo.Lookup(zoneID)
	if !ok {
		return "", "", fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
	}
	info, ok := reg.Get(serverID)
	if !ok {
		return "", "", fmt.Errorf("node %s: %w", serverID, types.ErrNotFound)
	}
	return serverID, info.Address(), nil
}
