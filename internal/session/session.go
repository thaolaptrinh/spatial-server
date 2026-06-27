package session

import (
	"sync"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

type Session struct {
	ClientID string
	PlayerID string
	ZoneID   types.ZoneID
	ServerID types.ServerID
	Closed   bool
}

func NewSession(clientID, playerID string, zoneID types.ZoneID, serverID types.ServerID) *Session {
	return &Session{
		ClientID: clientID,
		PlayerID: playerID,
		ZoneID:   zoneID,
		ServerID: serverID,
	}
}

func (s *Session) Close() {
	s.Closed = true
}

type Pool struct {
	mu   sync.RWMutex
	byID map[string]*Session
}

func NewPool() *Pool {
	return &Pool{byID: make(map[string]*Session)}
}

func (p *Pool) Add(s *Session) {
	p.mu.Lock()
	p.byID[s.ClientID] = s
	p.mu.Unlock()
}

func (p *Pool) Get(clientID string) (*Session, bool) {
	p.mu.RLock()
	s, ok := p.byID[clientID]
	p.mu.RUnlock()
	return s, ok
}

func (p *Pool) Remove(clientID string) {
	p.mu.Lock()
	delete(p.byID, clientID)
	p.mu.Unlock()
}

func (p *Pool) Count() int {
	p.mu.RLock()
	n := len(p.byID)
	p.mu.RUnlock()
	return n
}
