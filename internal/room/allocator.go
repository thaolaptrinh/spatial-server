package room

import (
	"github.com/thaolaptrinh/spatial-server/internal/types"
)

// Allocator selects a Runtime Node to own a new zone. Implementations encode
// scheduling policy. Only the minimum required policy (least-loaded) is
// implemented here; future policies (round-robin, capacity-aware, sticky,
// label-aware, region-aware) can be added as additional implementations
// without changing any caller.
type Allocator interface {
	// Select picks one node from candidates. It must not mutate the input.
	Select(candidates []*NodeDescriptor) (*NodeDescriptor, error)
}

// LeastLoadedAllocator selects the active, below-capacity node with the lowest
// load score. Load score orders primarily by zone count, secondarily by active
// entities, so a node with fewer owned zones (and fewer entities) is preferred.
type LeastLoadedAllocator struct{}

// Compile-time interface check.
var _ Allocator = LeastLoadedAllocator{}

// Select implements Allocator.
func (LeastLoadedAllocator) Select(candidates []*NodeDescriptor) (*NodeDescriptor, error) {
	var best *NodeDescriptor
	for _, n := range candidates {
		if n == nil || !n.CanAccept() {
			continue
		}
		if best == nil || loadScore(n) < loadScore(best) {
			best = n
		}
	}
	if best == nil {
		return nil, ErrNoCapacity
	}
	return best, nil
}

func loadScore(n *NodeDescriptor) float64 {
	return float64(n.Load.ZoneCount)*100000 + float64(n.Load.ActiveEntities)
}

// IsNodeStatus is a guard helper for callers filtering by status.
func IsNodeStatus(n *NodeDescriptor, s types.ServerStatus) bool { return n != nil && n.Status == s }
