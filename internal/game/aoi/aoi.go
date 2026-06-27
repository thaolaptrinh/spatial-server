package aoi

import (
	"math"
	"sort"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

type cellKey struct {
	x, y int
}

type AOI struct {
	cellSize  float64
	radius    float64
	cellRange int
	cells     map[cellKey][]types.EntityID
	positions map[types.EntityID]types.Vector3
}

func New(cellSize, radius float64) *AOI {
	cellRange := int(math.Ceil(radius / cellSize))
	return &AOI{
		cellSize:  cellSize,
		radius:    radius,
		cellRange: cellRange,
		cells:     make(map[cellKey][]types.EntityID),
		positions: make(map[types.EntityID]types.Vector3),
	}
}

func (a *AOI) cellKeyFor(pos types.Vector3) cellKey {
	return cellKey{
		x: int(math.Floor(pos.X / a.cellSize)),
		y: int(math.Floor(pos.Z / a.cellSize)),
	}
}

func (a *AOI) Enter(id types.EntityID, pos types.Vector3) {
	key := a.cellKeyFor(pos)
	a.cells[key] = append(a.cells[key], id)
	a.positions[id] = pos
}

func (a *AOI) Leave(id types.EntityID) {
	pos, ok := a.positions[id]
	if !ok {
		return
	}
	key := a.cellKeyFor(pos)
	ids := a.cells[key]
	for i, eid := range ids {
		if eid == id {
			a.cells[key] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	delete(a.positions, id)
}

func (a *AOI) Move(id types.EntityID, newPos types.Vector3) {
	oldPos, ok := a.positions[id]
	if !ok {
		a.Enter(id, newPos)
		return
	}
	oldKey := a.cellKeyFor(oldPos)
	newKey := a.cellKeyFor(newPos)
	if oldKey == newKey {
		a.positions[id] = newPos
		return
	}
	ids := a.cells[oldKey]
	for i, eid := range ids {
		if eid == id {
			a.cells[oldKey] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	a.cells[newKey] = append(a.cells[newKey], id)
	a.positions[id] = newPos
}

func (a *AOI) Query(pos types.Vector3) []types.EntityID {
	centerKey := a.cellKeyFor(pos)
	seen := make(map[types.EntityID]struct{})
	var result []types.EntityID

	for dx := -a.cellRange; dx <= a.cellRange; dx++ {
		for dy := -a.cellRange; dy <= a.cellRange; dy++ {
			key := cellKey{x: centerKey.x + dx, y: centerKey.y + dy}
			for _, id := range a.cells[key] {
				if _, ok := seen[id]; !ok {
					seen[id] = struct{}{}
					result = append(result, id)
				}
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

func (a *AOI) EntitiesInRange(center types.Vector3, radius float64) []types.EntityID {
	all := a.Query(center)
	radiusSq := radius * radius
	var result []types.EntityID
	for _, id := range all {
		pos := a.positions[id]
		dx := pos.X - center.X
		dz := pos.Z - center.Z
		if dx*dx+dz*dz <= radiusSq {
			result = append(result, id)
		}
	}
	return result
}

func (a *AOI) CellCoord(pos types.Vector3) (int, int) {
	ck := a.cellKeyFor(pos)
	return ck.x, ck.y
}

func (a *AOI) Count() int {
	return len(a.positions)
}
