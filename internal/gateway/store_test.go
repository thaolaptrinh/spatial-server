package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) (*RedisSessionStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisSessionStore(client, 60*time.Second), mr
}

func TestIssue_ReturnsTokenAndStores(t *testing.T) {
	store, mr := newTestStore(t)
	rec := SessionRecord{PlayerID: "p1", RuntimeID: "r1", ZoneID: "z1", GameServerAddr: "gs-1:9000", SourceIP: "1.2.3.4"}

	token, err := store.Issue(context.Background(), rec)
	require.NoError(t, err)
	assert.Len(t, token, 43)

	exists := mr.Exists(sessionKeyPrefix + token)
	assert.True(t, exists)
}

func TestLookup_HitResetsTTL(t *testing.T) {
	store, mr := newTestStore(t)
	rec := SessionRecord{PlayerID: "p1"}
	token, err := store.Issue(context.Background(), rec)
	require.NoError(t, err)

	mr.FastForward(45 * time.Second)

	got, ok, err := store.Lookup(context.Background(), token)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "p1", got.PlayerID)

	ttl := mr.TTL(sessionKeyPrefix + token)
	assert.Greater(t, ttl, 45*time.Second)
}

func TestLookup_MissAfterExpiry(t *testing.T) {
	store, mr := newTestStore(t)
	token, err := store.Issue(context.Background(), SessionRecord{PlayerID: "p1"})
	require.NoError(t, err)

	mr.FastForward(61 * time.Second)
	_, ok, err := store.Lookup(context.Background(), token)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRevoke_DeletesToken(t *testing.T) {
	store, mr := newTestStore(t)
	token, err := store.Issue(context.Background(), SessionRecord{PlayerID: "p1"})
	require.NoError(t, err)

	require.NoError(t, store.Revoke(context.Background(), token))
	assert.False(t, mr.Exists(sessionKeyPrefix + token))
}
