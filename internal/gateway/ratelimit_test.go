package gateway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTokenBucket_AllowsUpToBurst(t *testing.T) {
	b := newTokenBucket(100, 100, func() time.Time { return time.Unix(0, 0) })
	for i := 0; i < 100; i++ {
		assert.True(t, b.allow())
	}
	assert.False(t, b.allow())
}

func TestTokenBucket_RefillsOverTime(t *testing.T) {
	now := time.Unix(0, 0)
	clock := &now
	b := newTokenBucket(100, 100, func() time.Time { return *clock })
	for i := 0; i < 100; i++ {
		b.allow()
	}
	assert.False(t, b.allow())

	*clock = now.Add(1 * time.Second)
	assert.True(t, b.allow())
}

func TestConnectionLimiter_DropsAndCounts(t *testing.T) {
	l := newConnectionLimiter(100, 100, func() time.Time { return time.Unix(0, 0) })
	for i := 0; i < 100; i++ {
		assert.True(t, l.allow())
	}
	assert.False(t, l.allow())
	assert.Equal(t, uint64(1), l.drops.Load())
}

func TestIPLimiter_AggregatesAcrossConnections(t *testing.T) {
	now := time.Unix(0, 0)
	l := newIPLimiter(500, 500, func() time.Time { return now })
	for i := 0; i < 500; i++ {
		assert.True(t, l.allow("1.2.3.4"))
	}
	assert.False(t, l.allow("1.2.3.4"))
	assert.True(t, l.allow("5.6.7.8"))
}

func TestIPLimiter_DropsAndCounts(t *testing.T) {
	l := newIPLimiter(1, 1, func() time.Time { return time.Unix(0, 0) })
	assert.True(t, l.allow("1.2.3.4"))
	assert.False(t, l.allow("1.2.3.4"))
	assert.Equal(t, uint64(1), l.drops.Load())
}
