package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// GitHub Actions OIDC endpoints (FR-2.1).
const (
	ActionsIssuer  = "https://token.actions.githubusercontent.com"
	ActionsJWKSURL = ActionsIssuer + "/.well-known/jwks"
)

// ErrOIDCInvalid covers every reason an Actions OIDC token is refused. A JWKS
// that cannot be fetched is a different error, so the caller can answer 502.
var ErrOIDCInvalid = errors.New("oidc token invalid")

// nbfSkew tolerates runner clocks slightly ahead of the broker.
const nbfSkew = 60 * time.Second

// jwksRefetchEvery bounds how often an unknown kid may trigger a JWKS refetch.
const jwksRefetchEvery = time.Minute

// ActionsClaims are the claims the broker authorizes on. sub is deliberately
// absent: its format is customizable per repo; authorize by WorkflowRef.
type ActionsClaims struct {
	Repository, WorkflowRef, RunID string
}

// ActionsVerifier verifies GitHub Actions OIDC tokens (RS256, keys from the
// JWKS). Zero JWKSURL means ActionsJWKSURL; nil HTTP a client with a 10s
// timeout; nil Now time.Now.
type ActionsVerifier struct {
	JWKSURL string
	HTTP    *http.Client
	Now     func() time.Time

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

// Verify checks alg, signature, issuer, audience, exp and nbf and returns the
// workflow identity claims.
func (v *ActionsVerifier) Verify(ctx context.Context, token string) (ActionsClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ActionsClaims{}, ErrOIDCInvalid
	}
	var h struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	hb, err := b64.DecodeString(parts[0])
	if err != nil || json.Unmarshal(hb, &h) != nil || h.Alg != "RS256" || h.Kid == "" {
		return ActionsClaims{}, ErrOIDCInvalid
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return ActionsClaims{}, ErrOIDCInvalid
	}
	key, err := v.key(ctx, h.Kid)
	if err != nil {
		return ActionsClaims{}, err
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig) != nil {
		return ActionsClaims{}, ErrOIDCInvalid
	}
	var c struct {
		Iss         string          `json:"iss"`
		Aud         json.RawMessage `json:"aud"`
		Exp         int64           `json:"exp"`
		Nbf         int64           `json:"nbf"`
		Repository  string          `json:"repository"`
		WorkflowRef string          `json:"workflow_ref"`
		RunID       string          `json:"run_id"`
	}
	cb, err := b64.DecodeString(parts[1])
	if err != nil || json.Unmarshal(cb, &c) != nil {
		return ActionsClaims{}, ErrOIDCInvalid
	}
	now := v.now()
	if c.Iss != ActionsIssuer || !hasAudience(c.Aud) || c.Exp == 0 || now.Unix() >= c.Exp ||
		now.Add(nbfSkew).Unix() < c.Nbf || c.WorkflowRef == "" || c.Repository == "" {
		return ActionsClaims{}, ErrOIDCInvalid
	}
	return ActionsClaims{Repository: c.Repository, WorkflowRef: c.WorkflowRef, RunID: c.RunID}, nil
}

// hasAudience accepts aud as a string or an array of strings containing Audience.
func hasAudience(raw json.RawMessage) bool {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s == Audience
	}
	var arr []string
	return json.Unmarshal(raw, &arr) == nil && slices.Contains(arr, Audience)
}

func (v *ActionsVerifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

// key returns the public key for kid, fetching the JWKS when the cache is
// empty, or when kid is unknown and the last fetch is at least a minute old.
func (v *ActionsVerifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	if v.keys != nil && v.now().Sub(v.fetched) < jwksRefetchEvery {
		return nil, ErrOIDCInvalid
	}
	keys, err := v.fetch(ctx)
	if err != nil {
		return nil, err
	}
	v.keys, v.fetched = keys, v.now()
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, ErrOIDCInvalid
}

var defaultActionsClient = &http.Client{Timeout: 10 * time.Second}

// fetch downloads the JWKS. Its errors never wrap ErrOIDCInvalid.
func (v *ActionsVerifier) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	u := v.JWKSURL
	if u == "" {
		u = ActionsJWKSURL
	}
	c := v.HTTP
	if c == nil {
		c = defaultActionsClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("oidc jwks: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc jwks: status %d", resp.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kty, Kid, N, E string
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&set); err != nil {
		return nil, fmt.Errorf("oidc jwks: decode: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		n, errN := b64.DecodeString(k.N)
		e, errE := b64.DecodeString(k.E)
		if errN != nil || errE != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	return keys, nil
}
