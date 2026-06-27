package gateway

import (
	"sync"
	"sync/atomic"
	"time"
)

type tokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	rate     float64
	burst    float64
	lastTime time.Time
	now      func() time.Time
}

func newTokenBucket(rate, burst float64, now func() time.Time) *tokenBucket {
	return &tokenBucket{tokens: burst, rate: rate, burst: burst, lastTime: now(), now: now}
}

func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	elapsed := now.Sub(b.lastTime).Seconds()
	b.lastTime = now
	b.tokens += elapsed * b.rate
	if b.tokens > b.burst {
		b.tokens = b.burst
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

type connectionLimiter struct {
	bucket *tokenBucket
	drops  atomic.Uint64
}

func newConnectionLimiter(rate, burst float64, now func() time.Time) *connectionLimiter {
	return &connectionLimiter{bucket: newTokenBucket(rate, burst, now)}
}

func (l *connectionLimiter) allow() bool {
	if l.bucket.allow() {
		return true
	}
	l.drops.Add(1)
	return false
}

type ipLimiter struct {
	mu    sync.Map
	rate  float64
	burst float64
	now   func() time.Time
	drops atomic.Uint64
}

type ipBucket struct {
	bucket *tokenBucket
}

func newIPLimiter(rate, burst float64, now func() time.Time) *ipLimiter {
	return &ipLimiter{rate: rate, burst: burst, now: now}
}

func (l *ipLimiter) allow(ip string) bool {
	v, _ := l.mu.LoadOrStore(ip, &ipBucket{bucket: newTokenBucket(l.rate, l.burst, l.now)})
	b := v.(*ipBucket)
	if b.bucket.allow() {
		return true
	}
	l.drops.Add(1)
	return false
}
