package game

import (
	"sync"

	v1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type DeltaRingBuffer struct {
	mu    sync.Mutex
	buf   []*v1.EntityUpdate
	cap   int
	head  int
	count int
	drops uint64
}

func NewDeltaRingBuffer(capacity int) *DeltaRingBuffer {
	if capacity <= 0 {
		capacity = 1000
	}
	return &DeltaRingBuffer{
		buf: make([]*v1.EntityUpdate, capacity),
		cap: capacity,
	}
}

func (b *DeltaRingBuffer) Push(upd *v1.EntityUpdate) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.count == b.cap {
		b.buf[b.head] = upd
		b.head = (b.head + 1) % b.cap
		b.drops++
		return
	}
	idx := (b.head + b.count) % b.cap
	b.buf[idx] = upd
	b.count++
}

func (b *DeltaRingBuffer) Drain() []*v1.EntityUpdate {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]*v1.EntityUpdate, b.count)
	for i := 0; i < b.count; i++ {
		out[i] = b.buf[(b.head+i)%b.cap]
		b.buf[(b.head+i)%b.cap] = nil
	}
	b.head = 0
	b.count = 0
	return out
}

func (b *DeltaRingBuffer) Drops() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.drops
}
