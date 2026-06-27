package game

import (
	"fmt"
	"sync"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type peerConn struct {
	target types.ServerID
	addr   string
	conn   *grpc.ClientConn
	client spatialserverv1.GameServerClient
}

type PeerRegistry struct {
	mu    sync.RWMutex
	conns map[types.ServerID]*peerConn
}

func NewPeerRegistry() *PeerRegistry {
	return &PeerRegistry{conns: make(map[types.ServerID]*peerConn)}
}

func (p *PeerRegistry) Upsert(serverID types.ServerID, addr string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if pc, ok := p.conns[serverID]; ok {
		if pc.addr == addr {
			return nil
		}
		_ = pc.conn.Close()
		delete(p.conns, serverID)
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial peer %s: %w", serverID, err)
	}
	p.conns[serverID] = &peerConn{
		target: serverID,
		addr:   addr,
		conn:   conn,
		client: spatialserverv1.NewGameServerClient(conn),
	}
	return nil
}

func (p *PeerRegistry) Get(serverID types.ServerID) (spatialserverv1.GameServerClient, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pc, ok := p.conns[serverID]
	if !ok {
		return nil, false
	}
	return pc.client, true
}

func (p *PeerRegistry) Remove(serverID types.ServerID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if pc, ok := p.conns[serverID]; ok {
		_ = pc.conn.Close()
		delete(p.conns, serverID)
	}
}
