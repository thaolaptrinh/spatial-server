package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type jwksResponse struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		Crv string `json:"crv"`
		Alg string `json:"alg"`
		X   string `json:"x"`
	} `json:"keys"`
}

type JWKSProvider struct {
	url     string
	ttl     time.Duration
	mu      sync.RWMutex
	keys    map[string]any
	fetched time.Time
	client  *http.Client
}

func NewJWKSProvider(url string, ttl time.Duration) *JWKSProvider {
	return &JWKSProvider{
		url:    url,
		ttl:    ttl,
		keys:   make(map[string]any),
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (p *JWKSProvider) Refresh() error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, p.url, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("jwks fetch: %w", err)
	}
	defer resp.Body.Close()

	var body jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("jwks decode: %w", err)
	}

	keys := make(map[string]any)
	for _, k := range body.Keys {
		if k.Alg != "EdDSA" || k.Crv != "Ed25519" {
			continue
		}
		x, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil || len(x) != ed25519.PublicKeySize {
			continue
		}
		keys[k.Kid] = ed25519.PublicKey(x)
	}

	p.mu.Lock()
	p.keys = keys
	p.fetched = time.Now()
	p.mu.Unlock()

	return nil
}

func (p *JWKSProvider) Verifier(kid string) jwt.Keyfunc {
	return func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method %v (EdDSA required)", t.Header["alg"])
		}
		p.mu.RLock()
		defer p.mu.RUnlock()
		if time.Since(p.fetched) > p.ttl {
			return nil, fmt.Errorf("jwks cache stale past ttl, refusing to verify")
		}
		k, ok := p.keys[kid]
		if !ok {
			return nil, fmt.Errorf("unknown kid %s", kid)
		}
		return k, nil
	}
}
