package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPostgresPool_InvalidDSN(t *testing.T) {
	pool, err := NewPostgresPool(context.Background(), "invalid-dsn")
	assert.Error(t, err)
	assert.Nil(t, pool)
}

func TestNewRedisClient_InvalidAddr(t *testing.T) {
	client, err := NewRedisClient("invalid-addr:0")
	assert.Error(t, err)
	assert.Nil(t, client)
}
