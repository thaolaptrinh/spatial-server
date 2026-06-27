package auth

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	RuntimeID string `json:"runtime_id"`
	PlayerID  string `json:"player_id"`
	ZoneID    string `json:"zone_id"`
}

func ValidateToken(tokenStr string, secret []byte) (*Claims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return &Claims{
		RuntimeID: getString(claims, "runtime_id"),
		PlayerID:  getString(claims, "player_id"),
		ZoneID:    getString(claims, "zone_id"),
	}, nil
}

func getString(claims jwt.MapClaims, key string) string {
	v, ok := claims[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
