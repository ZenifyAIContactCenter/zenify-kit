package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func protected() http.Handler {
	return Require(testKey, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hi " + Login(r.Context())))
	}))
}

func TestRequire_401WithoutBearer(t *testing.T) {
	expired, _, _ := Issue(testKey, "octocat", time.Now().Add(-time.Hour))
	other, _, _ := Issue([]byte("other-key-0123456789abcdef012345678"), "octocat", time.Now())
	for name, hdr := range map[string]string{
		"none":    "",
		"basic":   "Basic dXNlcjpwYXNz",
		"garbage": "Bearer not-a-jwt",
		"expired": "Bearer " + expired,
		"badsig":  "Bearer " + other,
	} {
		req := httptest.NewRequest(http.MethodPost, "/query", nil)
		if hdr != "" {
			req.Header.Set("Authorization", hdr)
		}
		rec := httptest.NewRecorder()
		protected().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, rec.Code)
			continue
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] != "unauthorized" {
			t.Errorf("%s: body = %s", name, rec.Body.String())
		}
	}
}

func TestRequire_PassesLogin(t *testing.T) {
	tok, _, _ := Issue(testKey, "octocat", time.Now())
	for _, scheme := range []string{"Bearer ", "bearer ", "BEARER "} {
		req := httptest.NewRequest(http.MethodPost, "/query", nil)
		req.Header.Set("Authorization", scheme+tok)
		rec := httptest.NewRecorder()
		protected().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != "hi octocat" {
			t.Errorf("%q: got %d %q", scheme, rec.Code, rec.Body.String())
		}
	}
}
