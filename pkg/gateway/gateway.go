package gateway

import (
	"sync"
	"time"
)

type CacheEntry struct {
	ServerID string
	Host     string
	Port     int
}

type cacheItem struct {
	entry     CacheEntry
	expiresAt time.Time
}

type RouterCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]*cacheItem
}

func NewRouterCache(ttl time.Duration) *RouterCache {
	return &RouterCache{
		ttl:     ttl,
		entries: make(map[string]*cacheItem),
	}
}

func (rc *RouterCache) Set(zoneID, serverID, host string, port int) {
	rc.mu.Lock()
	rc.entries[zoneID] = &cacheItem{
		entry:     CacheEntry{ServerID: serverID, Host: host, Port: port},
		expiresAt: time.Now().Add(rc.ttl),
	}
	rc.mu.Unlock()
}

func (rc *RouterCache) Get(zoneID string) (CacheEntry, bool) {
	rc.mu.RLock()
	item, ok := rc.entries[zoneID]
	if !ok {
		rc.mu.RUnlock()
		return CacheEntry{}, false
	}
	expired := time.Now().After(item.expiresAt)
	rc.mu.RUnlock()

	if expired {
		rc.mu.Lock()
		delete(rc.entries, zoneID)
		rc.mu.Unlock()
		return CacheEntry{}, false
	}
	return item.entry, true
}
