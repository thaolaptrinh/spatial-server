package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWKSProvider_VerifiesEdDSAToken(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	x := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{"sub": "test"})
	x.Header["kid"] = "k1"
	tokenStr, err := x.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string][]map[string]string{
			"keys": {
				{"kid": "k1", "kty": "OKP", "crv": "Ed25519", "alg": "EdDSA", "x": base64.RawURLEncoding.EncodeToString(pub)},
			},
		})
	}))
	defer srv.Close()

	p := NewJWKSProvider(srv.URL, time.Minute)
	if err := p.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	_, err = jwt.Parse(tokenStr, p.Verifier("k1"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
}
