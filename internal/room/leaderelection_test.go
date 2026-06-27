package room

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLeadershipGate_OnlyLeaderServesWrites(t *testing.T) {
	fake := &FakeLease{}
	fake.held.Store(false)
	gate := NewLeadershipGate(fake)
	assert.False(t, gate.IsLeader())

	var writes atomic.Int32
	gate.DoIfLeader(func() { writes.Add(1) })
	assert.Equal(t, int32(0), writes.Load())

	fake.held.Store(true)
	assert.True(t, gate.IsLeader())
	gate.DoIfLeader(func() { writes.Add(1) })
	assert.Equal(t, int32(1), writes.Load())
}
