package auth

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// oidcFixture serves a JWKS holding one generated RSA key and counts fetches.
type oidcFixture struct {
	key     *rsa.PrivateKey
	srv     *httptest.Server
	fetches atomic.Int32
	fail    atomic.Bool   // answer 503
	block   atomic.Bool   // hold the response until release is closed
	release chan struct{} // closed by the test to let blocked fetches finish
}

func newOIDCFixture(t *testing.T) *oidcFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &oidcFixture{key: key, release: make(chan struct{})}
	enc := base64.RawURLEncoding
	jwks := map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
		"n": enc.EncodeToString(key.N.Bytes()),
		"e": enc.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.fetches.Add(1)
		if f.block.Load() {
			<-f.release
		}
		if f.fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *oidcFixture) verifier(now time.Time) *ActionsVerifier {
	return &ActionsVerifier{JWKSURL: f.srv.URL, Now: func() time.Time { return now }}
}

// rs256 signs header.claims with key using RSASSA-PKCS1-v1_5 / SHA-256.
func rs256(t *testing.T, key *rsa.PrivateKey, header, claims string) string {
	t.Helper()
	enc := base64.RawURLEncoding
	in := enc.EncodeToString([]byte(header)) + "." + enc.EncodeToString([]byte(claims))
	sum := sha256.Sum256([]byte(in))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return in + "." + enc.EncodeToString(sig)
}

const (
	okHeader = `{"alg":"RS256","typ":"JWT","kid":"k1"}`
	wfRef    = "ZenifyAIContactCenter/contact-center-web/.github/workflows/e2e-staging.yml@refs/heads/staging"
)

// claimsJSON builds GitHub-shaped claims around t0 (2026-10-15T08:00:00Z = 1792051200).
func claimsJSON(mut func(m map[string]any)) string {
	m := map[string]any{
		"iss":          "https://token.actions.githubusercontent.com",
		"aud":          "zenify-broker",
		"sub":          "repo:ZenifyAIContactCenter/contact-center-web:ref:refs/heads/staging",
		"repository":   "ZenifyAIContactCenter/contact-center-web",
		"workflow_ref": wfRef,
		"run_id":       "123456789",
		"iat":          1792051200 - 10,
		"nbf":          1792051200 - 10,
		"exp":          1792051200 + 300,
	}
	if mut != nil {
		mut(m)
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func TestActionsVerify_Valid(t *testing.T) {
	f := newOIDCFixture(t)
	tok := rs256(t, f.key, okHeader, claimsJSON(nil))
	c, err := f.verifier(t0).Verify(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	want := ActionsClaims{Repository: "ZenifyAIContactCenter/contact-center-web", WorkflowRef: wfRef, RunID: "123456789"}
	if c != want {
		t.Errorf("claims = %+v, want %+v", c, want)
	}
	// aud as an array containing the broker audience is accepted too.
	tok = rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { m["aud"] = []string{"other", "zenify-broker"} }))
	if _, err := f.verifier(t0).Verify(context.Background(), tok); err != nil {
		t.Errorf("aud array: %v", err)
	}
}

func TestActionsVerify_Rejects(t *testing.T) {
	f := newOIDCFixture(t)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding
	valid := rs256(t, f.key, okHeader, claimsJSON(nil))
	parts := splitJWT(valid)
	hs := hmacToken(okHeaderAlg("HS256"), claimsJSON(nil))
	cases := map[string]string{
		"signed by other key":              rs256(t, other, okHeader, claimsJSON(nil)),
		"alg HS256":                        hs,
		"alg none":                         enc.EncodeToString([]byte(okHeaderAlg("none"))) + "." + parts[1] + ".",
		"alg none keeps RS sig":            enc.EncodeToString([]byte(okHeaderAlg("none"))) + "." + parts[1] + "." + parts[2],
		"alg none header, real RS256 sig":  rs256(t, f.key, okHeaderAlg("none"), claimsJSON(nil)),
		"alg HS256 header, real RS256 sig": rs256(t, f.key, okHeaderAlg("HS256"), claimsJSON(nil)),
		"wrong iss":                        rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { m["iss"] = "https://evil.example" })),
		"github default aud":               rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { m["aud"] = "https://github.com/ZenifyAIContactCenter" })),
		"aud array without broker":         rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { m["aud"] = []string{"a", "b"} })),
		"expired":                          rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { m["exp"] = 1792051200 - 1 })),
		"no exp":                           rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { delete(m, "exp") })),
		"nbf in future":                    rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { m["nbf"] = 1792051200 + 120 })),
		"unknown kid":                      rs256(t, f.key, `{"alg":"RS256","typ":"JWT","kid":"nope"}`, claimsJSON(nil)),
		"no repository":                    rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { delete(m, "repository") })),
		"now == exp":                       rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { m["exp"] = 1792051200 })),
		"no workflow_ref":                  rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { delete(m, "workflow_ref") })),
		"tampered claims":                  parts[0] + "." + enc.EncodeToString([]byte(claimsJSON(func(m map[string]any) { m["workflow_ref"] = "x/y/.github/workflows/z.yml@refs/heads/main" }))) + "." + parts[2],
		"two parts":                        parts[0] + "." + parts[1],
		"empty":                            "",
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := f.verifier(t0).Verify(context.Background(), tok)
			if !errors.Is(err, ErrOIDCInvalid) {
				t.Errorf("err = %v, want ErrOIDCInvalid", err)
			}
		})
	}
	// nbf inside the 60s skew is still accepted.
	tok := rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { m["nbf"] = 1792051200 + 30 }))
	if _, err := f.verifier(t0).Verify(context.Background(), tok); err != nil {
		t.Errorf("nbf within skew: %v", err)
	}
}

func okHeaderAlg(alg string) string { return `{"alg":"` + alg + `","typ":"JWT","kid":"k1"}` }

func splitJWT(tok string) []string { return strings.Split(tok, ".") }

// hmacToken signs with HS256 — the alg-confusion attack; it must never be
// accepted whatever secret was used.
func hmacToken(header, claims string) string {
	enc := base64.RawURLEncoding
	in := enc.EncodeToString([]byte(header)) + "." + enc.EncodeToString([]byte(claims))
	m := hmac.New(sha256.New, []byte("public-key-bytes-used-as-hmac-secret!!"))
	m.Write([]byte(in))
	return in + "." + enc.EncodeToString(m.Sum(nil))
}

func TestActionsVerify_CachesJWKS(t *testing.T) {
	f := newOIDCFixture(t)
	now := t0
	v := &ActionsVerifier{JWKSURL: f.srv.URL, Now: func() time.Time { return now }}
	ctx := context.Background()
	tok := rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { m["exp"] = 1792051200 + 3600 }))
	unknown := rs256(t, f.key, `{"alg":"RS256","typ":"JWT","kid":"rotated"}`, claimsJSON(func(m map[string]any) { m["exp"] = 1792051200 + 3600 }))

	for i := 0; i < 2; i++ {
		if _, err := v.Verify(ctx, tok); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.fetches.Load(); n != 1 {
		t.Fatalf("fetches after two verifies = %d, want 1", n)
	}
	// Unknown kid within a minute of the last fetch: no refetch.
	if _, err := v.Verify(ctx, unknown); !errors.Is(err, ErrOIDCInvalid) {
		t.Fatalf("unknown kid err = %v", err)
	}
	if n := f.fetches.Load(); n != 1 {
		t.Fatalf("fetches after early unknown kid = %d, want 1", n)
	}
	// After a minute an unknown kid refetches, once.
	now = t0.Add(61 * time.Second)
	_, _ = v.Verify(ctx, unknown)
	_, _ = v.Verify(ctx, unknown)
	if n := f.fetches.Load(); n != 2 {
		t.Fatalf("fetches after late unknown kid = %d, want 2", n)
	}
	// Known kid never refetches.
	now = t0.Add(10 * time.Minute)
	if _, err := v.Verify(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if n := f.fetches.Load(); n != 2 {
		t.Fatalf("fetches after known kid = %d, want 2", n)
	}
}

func TestActionsVerify_JWKSDown(t *testing.T) {
	f := newOIDCFixture(t)
	tok := rs256(t, f.key, okHeader, claimsJSON(nil))
	f.srv.Close()
	_, err := f.verifier(t0).Verify(context.Background(), tok)
	if err == nil || errors.Is(err, ErrOIDCInvalid) {
		t.Errorf("err = %v, want a non-ErrOIDCInvalid error", err)
	}
	// A JWKS endpoint answering 5xx is also an upstream failure.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer srv.Close()
	v := &ActionsVerifier{JWKSURL: srv.URL, Now: func() time.Time { return t0 }}
	if _, err := v.Verify(context.Background(), tok); err == nil || errors.Is(err, ErrOIDCInvalid) {
		t.Errorf("503: err = %v, want a non-ErrOIDCInvalid error", err)
	}
}

func TestActionsVerify_FailedFetchNotRetriedWithinMinute(t *testing.T) {
	f := newOIDCFixture(t)
	f.fail.Store(true)
	now := t0
	v := &ActionsVerifier{JWKSURL: f.srv.URL, Now: func() time.Time { return now }}
	tok := rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { m["exp"] = 1792051200 + 3600 }))
	for i := 0; i < 3; i++ {
		if _, err := v.Verify(context.Background(), tok); err == nil || errors.Is(err, ErrOIDCInvalid) {
			t.Fatalf("verify %d: err = %v, want upstream error", i, err)
		}
	}
	if n := f.fetches.Load(); n != 1 {
		t.Fatalf("fetches after failed fetch = %d, want 1", n)
	}
	// After the window the fetch is retried and, upstream healthy again, succeeds.
	f.fail.Store(false)
	now = t0.Add(61 * time.Second)
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("after window: %v", err)
	}
	if n := f.fetches.Load(); n != 2 {
		t.Fatalf("fetches after window = %d, want 2", n)
	}
}

func TestActionsVerify_CachedKidDoesNotWaitOnDownload(t *testing.T) {
	f := newOIDCFixture(t)
	var now atomic.Int64
	now.Store(t0.Unix())
	v := &ActionsVerifier{JWKSURL: f.srv.URL, Now: func() time.Time { return time.Unix(now.Load(), 0) }}
	ctx := context.Background()
	tok := rs256(t, f.key, okHeader, claimsJSON(func(m map[string]any) { m["exp"] = 1792051200 + 3600 }))
	if _, err := v.Verify(ctx, tok); err != nil {
		t.Fatal(err)
	}
	// Start an unknown-kid refetch that hangs in the JWKS server.
	f.block.Store(true)
	now.Store(t0.Add(61 * time.Second).Unix())
	unknown := rs256(t, f.key, `{"alg":"RS256","typ":"JWT","kid":"rotated"}`, claimsJSON(func(m map[string]any) { m["exp"] = 1792051200 + 3600 }))
	hung := make(chan struct{})
	go func() { _, _ = v.Verify(ctx, unknown); close(hung) }()
	for f.fetches.Load() < 2 {
		time.Sleep(time.Millisecond)
	}
	done := make(chan error, 1)
	go func() { _, err := v.Verify(ctx, tok); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("cached kid: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("cached kid verify waited on the JWKS download")
	}
	close(f.release)
	<-hung
}
