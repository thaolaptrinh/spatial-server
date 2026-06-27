package game

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

func TestDeltaRingBuffer_PushAndDrainInOrder(t *testing.T) {
	b := NewDeltaRingBuffer(3)
	b.Push(&v1.EntityUpdate{EntityId: "e", Sequence: 1})
	b.Push(&v1.EntityUpdate{EntityId: "e", Sequence: 2})
	b.Push(&v1.EntityUpdate{EntityId: "e", Sequence: 3})

	out := b.Drain()
	require.Len(t, out, 3)
	assert.Equal(t, int32(1), out[0].GetSequence())
	assert.Equal(t, int32(3), out[2].GetSequence())
}

func TestDeltaRingBuffer_OverwritesOldestAndCountsDrops(t *testing.T) {
	b := NewDeltaRingBuffer(2)
	b.Push(&v1.EntityUpdate{Sequence: 1})
	b.Push(&v1.EntityUpdate{Sequence: 2})
	b.Push(&v1.EntityUpdate{Sequence: 3})

	out := b.Drain()
	require.Len(t, out, 2)
	assert.Equal(t, int32(2), out[0].GetSequence())
	assert.Equal(t, int32(3), out[1].GetSequence())
	assert.Equal(t, uint64(1), b.Drops())
}

func TestDeltaRingBuffer_DrainResets(t *testing.T) {
	b := NewDeltaRingBuffer(5)
	b.Push(&v1.EntityUpdate{Sequence: 1})
	_ = b.Drain()
	assert.Len(t, b.Drain(), 0)
}
