package room

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type RuntimeRecord struct {
	ID         string
	Status     types.RuntimeStatus
	ZoneCount  int
	ZoneSize   float64
	ZoneIDs    []string
	PlayerCount int
	CreatedAt  time.Time
}

type RuntimeStore interface {
	Create(ctx context.Context, id string, zoneCount int, zoneSize float64) (*RuntimeRecord, error)
	Get(ctx context.Context, id string) (*RuntimeRecord, error)
	Destroy(ctx context.Context, id string) error
	List(ctx context.Context, pageToken string, pageSize int) ([]*RuntimeRecord, string, error)
}

type SpatialServerAPI struct {
	spatialserverv1.UnimplementedSpatialServerAPIServer
	store        RuntimeStore
	gatewayAddr  string
	defaultZones int
	defaultSize  float64
}

func NewSpatialServerAPI(store RuntimeStore, gatewayAddr string) *SpatialServerAPI {
	return &SpatialServerAPI{
		store:        store,
		gatewayAddr:  gatewayAddr,
		defaultZones: 1,
		defaultSize:  100.0,
	}
}

func (s *SpatialServerAPI) CreateRuntime(ctx context.Context, req *spatialserverv1.CreateRuntimeRequest) (*spatialserverv1.CreateRuntimeResponse, error) {
	if req.RuntimeId == "" {
		return nil, status.Error(codes.InvalidArgument, "runtime_id is required")
	}
	zc := req.ZoneCount
	if zc <= 0 {
		zc = int32(s.defaultZones)
	}
	zs := req.ZoneSize
	if zs <= 0 {
		zs = s.defaultSize
	}
	rec, err := s.store.Create(ctx, req.RuntimeId, int(zc), zs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create runtime: %v", err)
	}
	zones := make([]*spatialserverv1.ZoneInfo, rec.ZoneCount)
	for i := 0; i < rec.ZoneCount; i++ {
		zones[i] = &spatialserverv1.ZoneInfo{
			ZoneId: fmt.Sprintf("%s-z%d", rec.ID, i+1),
		}
	}
	return &spatialserverv1.CreateRuntimeResponse{
		RuntimeId:   rec.ID,
		GatewayAddr: s.gatewayAddr,
		ZoneCount:   int32(rec.ZoneCount),
		Zones:       zones,
	}, nil
}

func (s *SpatialServerAPI) DestroyRuntime(ctx context.Context, req *spatialserverv1.DestroyRuntimeRequest) (*spatialserverv1.DestroyRuntimeResponse, error) {
	if err := s.store.Destroy(ctx, req.RuntimeId); err != nil {
		if err == types.ErrNotFound {
			return nil, status.Error(codes.NotFound, "runtime not found")
		}
		return nil, status.Errorf(codes.Internal, "destroy runtime: %v", err)
	}
	return &spatialserverv1.DestroyRuntimeResponse{Success: true}, nil
}

func (s *SpatialServerAPI) GetRuntimeInfo(ctx context.Context, req *spatialserverv1.GetRuntimeInfoRequest) (*spatialserverv1.GetRuntimeInfoResponse, error) {
	rec, err := s.store.Get(ctx, req.RuntimeId)
	if err != nil {
		if err == types.ErrNotFound {
			return nil, status.Error(codes.NotFound, "runtime not found")
		}
		return nil, status.Errorf(codes.Internal, "get runtime: %v", err)
	}
	return &spatialserverv1.GetRuntimeInfoResponse{
		RuntimeId:   rec.ID,
		Status:      toProtoStatus(rec.Status),
		ZoneCount:   int32(rec.ZoneCount),
		PlayerCount: int32(rec.PlayerCount),
		CreatedAt:   rec.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *SpatialServerAPI) GetRuntimeMetrics(ctx context.Context, req *spatialserverv1.GetRuntimeMetricsRequest) (*spatialserverv1.GetRuntimeMetricsResponse, error) {
	rec, err := s.store.Get(ctx, req.RuntimeId)
	if err != nil {
		if err == types.ErrNotFound {
			return nil, status.Error(codes.NotFound, "runtime not found")
		}
		return nil, status.Errorf(codes.Internal, "get runtime metrics: %v", err)
	}
	return &spatialserverv1.GetRuntimeMetricsResponse{
		RuntimeId:   rec.ID,
		PlayerCount: int32(rec.PlayerCount),
	}, nil
}

func (s *SpatialServerAPI) ListRuntimes(ctx context.Context, req *spatialserverv1.ListRuntimesRequest) (*spatialserverv1.ListRuntimesResponse, error) {
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	records, nextToken, err := s.store.List(ctx, req.PageToken, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list runtimes: %v", err)
	}
	runtimes := make([]*spatialserverv1.GetRuntimeInfoResponse, len(records))
	for i, rec := range records {
		runtimes[i] = &spatialserverv1.GetRuntimeInfoResponse{
			RuntimeId:   rec.ID,
			Status:      toProtoStatus(rec.Status),
			ZoneCount:   int32(rec.ZoneCount),
			PlayerCount: int32(rec.PlayerCount),
			CreatedAt:   rec.CreatedAt.Format(time.RFC3339),
		}
	}
	return &spatialserverv1.ListRuntimesResponse{
		Runtimes:      runtimes,
		NextPageToken: nextToken,
	}, nil
}

func toProtoStatus(s types.RuntimeStatus) spatialserverv1.RuntimeStatus {
	switch s {
	case types.RuntimeStatusCreating:
		return spatialserverv1.RuntimeStatus_RUNTIME_STATUS_CREATING
	case types.RuntimeStatusActive:
		return spatialserverv1.RuntimeStatus_RUNTIME_STATUS_ACTIVE
	case types.RuntimeStatusDraining:
		return spatialserverv1.RuntimeStatus_RUNTIME_STATUS_DRAINING
	case types.RuntimeStatusDestroyed:
		return spatialserverv1.RuntimeStatus_RUNTIME_STATUS_DESTROYED
	default:
		return spatialserverv1.RuntimeStatus_RUNTIME_STATUS_UNSPECIFIED
	}
}

type MemoryRuntimeStore struct {
	mu   sync.RWMutex
	data map[string]*RuntimeRecord
}

func NewMemoryRuntimeStore() *MemoryRuntimeStore {
	return &MemoryRuntimeStore{data: make(map[string]*RuntimeRecord)}
}

func (m *MemoryRuntimeStore) Create(_ context.Context, id string, zoneCount int, zoneSize float64) (*RuntimeRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[id]; ok {
		return nil, types.ErrConflict
	}
	r := &RuntimeRecord{
		ID:        id,
		Status:    types.RuntimeStatusCreating,
		ZoneCount: zoneCount,
		ZoneSize:  zoneSize,
		CreatedAt: time.Now(),
	}
	for i := 0; i < zoneCount; i++ {
		r.ZoneIDs = append(r.ZoneIDs, fmt.Sprintf("%s-z%d", id, i+1))
	}
	r.Status = types.RuntimeStatusActive
	m.data[id] = r
	return r, nil
}

func (m *MemoryRuntimeStore) Get(_ context.Context, id string) (*RuntimeRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.data[id]
	if !ok {
		return nil, types.ErrNotFound
	}
	return r, nil
}

func (m *MemoryRuntimeStore) Destroy(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.data[id]
	if !ok {
		return types.ErrNotFound
	}
	r.Status = types.RuntimeStatusDestroyed
	return nil
}

func (m *MemoryRuntimeStore) List(_ context.Context, _ string, _ int) ([]*RuntimeRecord, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*RuntimeRecord
	for _, r := range m.data {
		if r.Status != types.RuntimeStatusDestroyed {
			out = append(out, r)
		}
	}
	return out, "", nil
}
