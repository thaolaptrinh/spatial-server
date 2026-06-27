package gateway

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"
)

type SessionRecord struct {
	PlayerID       string    `json:"player_id"`
	RuntimeID      string    `json:"runtime_id"`
	ZoneID         string    `json:"zone_id"`
	GameServerAddr string    `json:"game_server_addr"`
	CreatedAt      time.Time `json:"created_at"`
	LastActivity   time.Time `json:"last_activity"`
	SourceIP       string    `json:"source_ip"`
}

func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
