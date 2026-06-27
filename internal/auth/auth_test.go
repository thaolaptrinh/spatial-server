package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func TestValidateToken_Valid(t *testing.T) {
	secret := []byte("test-secret")
	runtimeID := "rt-123"
	playerID := "player-1"
	zoneID := "zone-1"

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"runtime_id": runtimeID,
		"player_id":  playerID,
		"zone_id":    zoneID,
		"exp":        time.Now().Add(1 * time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString(secret)

	claims, err := ValidateToken(tokenStr, secret)
	assert.NoError(t, err)
	assert.Equal(t, runtimeID, claims.RuntimeID)
	assert.Equal(t, playerID, claims.PlayerID)
	assert.Equal(t, zoneID, claims.ZoneID)
}

func TestValidateToken_Expired(t *testing.T) {
	secret := []byte("test-secret")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(-1 * time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString(secret)

	_, err := ValidateToken(tokenStr, secret)
	assert.Error(t, err)
}

func TestValidateToken_WrongSecret(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString([]byte("correct-secret"))

	_, err := ValidateToken(tokenStr, []byte("wrong-secret"))
	assert.Error(t, err)
}

func TestValidateToken_InvalidSignature(t *testing.T) {
	_, err := ValidateToken("not-a-token", []byte("secret"))
	assert.Error(t, err)
}
