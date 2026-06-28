package security

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestExpiredToken_Rejected(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "test",
		"exp": time.Now().Add(-1 * time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		return []byte("secret"), nil
	})
	if err == nil {
		t.Fatal("expected expired token to be rejected")
	}
	t.Logf("expired token correctly rejected: %v", err)
}

func TestWrongSigningKey_Rejected(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "test"})
	tokenStr, err := token.SignedString([]byte("correct-secret"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		return []byte("wrong-secret"), nil
	})
	if err == nil {
		t.Fatal("expected wrong-signing-key to be rejected")
	}
	t.Logf("wrong signing key correctly rejected: %v", err)
}
