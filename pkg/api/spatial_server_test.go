package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thaolaptrinh/spatial-server/internal/types"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

func TestCreateRuntime_AndDestroy(t *testing.T) {
	srv := NewSpatialServerAPI(&fakeStore{}, "gw:443")
	resp, err := srv.CreateRuntime(context.Background(), &spatialserverv1.CreateRuntimeRequest{RuntimeId: "r1", ZoneCount: 2})
	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.ZoneCount)
	assert.Len(t, resp.Zones, 2)
	d, err := srv.DestroyRuntime(context.Background(), &spatialserverv1.DestroyRuntimeRequest{RuntimeId: "r1"})
	require.NoError(t, err)
	assert.True(t, d.Success)
}

type fakeStore struct{ m map[string]*RuntimeRecord }

func (f *fakeStore) Create(_ context.Context, id string, zc int, _ float64) (*RuntimeRecord, error) {
	if f.m == nil {
		f.m = map[string]*RuntimeRecord{}
	}
	r := &RuntimeRecord{ID: id, Status: types.RuntimeStatusActive, ZoneCount: zc}
	for i := 0; i < zc; i++ {
		r.ZoneIDs = append(r.ZoneIDs, id+"-z"+string(rune('1'+i)))
	}
	f.m[id] = r
	return r, nil
}

func (f *fakeStore) Get(_ context.Context, id string) (*RuntimeRecord, error) {
	if r, ok := f.m[id]; ok {
		return r, nil
	}
	return nil, types.ErrNotFound
}

func (f *fakeStore) Destroy(_ context.Context, id string) error {
	if r, ok := f.m[id]; ok {
		r.Status = types.RuntimeStatusDestroyed
		return nil
	}
	return types.ErrNotFound
}

func (f *fakeStore) List(_ context.Context, _ string, _ int) ([]*RuntimeRecord, string, error) {
	var out []*RuntimeRecord
	for _, r := range f.m {
		if r.Status != types.RuntimeStatusDestroyed {
			out = append(out, r)
		}
	}
	return out, "", nil
}
