package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const sessionKeyPrefix = "session:"

type SessionStore interface {
	Issue(ctx context.Context, rec SessionRecord) (string, error)
	Lookup(ctx context.Context, token string) (SessionRecord, bool, error)
	Touch(ctx context.Context, token string) error
	Revoke(ctx context.Context, token string) error
}

type RedisSessionStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisSessionStore(client *redis.Client, ttl time.Duration) *RedisSessionStore {
	return &RedisSessionStore{client: client, ttl: ttl}
}

func (s *RedisSessionStore) Issue(ctx context.Context, rec SessionRecord) (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", err
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	rec.LastActivity = rec.CreatedAt
	data, err := json.Marshal(rec)
	if err != nil {
		return "", fmt.Errorf("marshal session record: %w", err)
	}
	if err := s.client.Set(ctx, sessionKeyPrefix+token, data, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("redis set session: %w", err)
	}
	return token, nil
}

func (s *RedisSessionStore) Lookup(ctx context.Context, token string) (SessionRecord, bool, error) {
	raw, err := s.client.Get(ctx, sessionKeyPrefix+token).Bytes()
	if err == redis.Nil {
		return SessionRecord{}, false, nil
	}
	if err != nil {
		return SessionRecord{}, false, fmt.Errorf("redis get session: %w", err)
	}
	var rec SessionRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return SessionRecord{}, false, fmt.Errorf("unmarshal session record: %w", err)
	}
	if err := s.Touch(ctx, token); err != nil {
		return SessionRecord{}, false, err
	}
	return rec, true, nil
}

func (s *RedisSessionStore) Touch(ctx context.Context, token string) error {
	if err := s.client.Expire(ctx, sessionKeyPrefix+token, s.ttl).Err(); err != nil {
		return fmt.Errorf("redis expire session: %w", err)
	}
	return nil
}

func (s *RedisSessionStore) Revoke(ctx context.Context, token string) error {
	if err := s.client.Del(ctx, sessionKeyPrefix+token).Err(); err != nil {
		return fmt.Errorf("redis del session: %w", err)
	}
	return nil
}
