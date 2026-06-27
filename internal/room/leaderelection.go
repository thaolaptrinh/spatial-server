package room

import (
	"sync"
	"sync/atomic"
	"time"
)

type Lease interface {
	Acquire() bool
	Renew() bool
	Release()
	IsHeld() bool
	Run(renewInterval time.Duration)
}

type LeadershipGate struct {
	mu    sync.RWMutex
	lease Lease
}

func NewLeadershipGate(l Lease) *LeadershipGate {
	return &LeadershipGate{lease: l}
}

func (g *LeadershipGate) SetLease(l Lease) {
	g.mu.Lock()
	g.lease = l
	g.mu.Unlock()
}

func (g *LeadershipGate) IsLeader() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.lease != nil && g.lease.IsHeld()
}

func (g *LeadershipGate) DoIfLeader(fn func()) {
	if !g.IsLeader() {
		return
	}
	fn()
}

type FakeLease struct {
	held atomic.Bool
}

func (f *FakeLease) Acquire() bool {
	f.held.Store(true)
	return true
}

func (f *FakeLease) Renew() bool {
	return f.held.Load()
}

func (f *FakeLease) Release() {
	f.held.Store(false)
}

func (f *FakeLease) IsHeld() bool {
	return f.held.Load()
}

func (f *FakeLease) Run(renewInterval time.Duration) {}
