package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	adminPW = "admin-secret-pw"
	userPW  = "user-secret-pw"
)

// fakeUpstream plays hub (/auth/login) and be (/v2/...). Shapes follow be's
// ok.js envelope and list-agents payload.
type fakeUpstream struct {
	adminToken  atomic.Value // string: the token be currently accepts
	adminLogins int32
	got         map[string]map[string]any // path -> last JSON body
	gotAuth     map[string]string
	gotQuery    string
}

func newUpstream(t *testing.T) (*fakeUpstream, *HTTPApp) {
	t.Helper()
	f := &fakeUpstream{got: map[string]map[string]any{}, gotAuth: map[string]string{}}
	f.adminToken.Store("admin-jwt-1")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		f.got[r.URL.Path] = b
		switch b["email"] {
		case "admin@zenify.test":
			if b["password"] != adminPW {
				w.WriteHeader(401)
				return
			}
			atomic.AddInt32(&f.adminLogins, 1)
			_, _ = io.WriteString(w, `{"token":"`+f.adminToken.Load().(string)+`"}`)
		case "top@e2e.test":
			if b["password"] != userPW {
				w.WriteHeader(400)
				_, _ = io.WriteString(w, `{"message":"wrong password `+userPW+`"}`)
				return
			}
			_, _ = io.WriteString(w, `{"token":"top-jwt"}`)
		case "nested@e2e.test":
			_, _ = io.WriteString(w, `{"data":{"token":"nested-jwt"}}`)
		default:
			w.WriteHeader(503)
		}
	})
	admin := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			f.gotAuth[r.URL.Path] = r.Header.Get("Authorization")
			if r.Header.Get("Authorization") != "Bearer "+f.adminToken.Load().(string) {
				w.WriteHeader(401)
				return
			}
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			f.got[r.URL.Path] = b
			h(w, r)
		}
	}
	mux.HandleFunc("GET /v2/admin/settings/t1/list-agents", admin(func(w http.ResponseWriter, r *http.Request) {
		f.gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `{"error":false,"status":200,"data":{"list":[
			{"_id":"652f00000000000000000001","email":"e2e-alice-agent2@e2e.test"},
			{"_id":"652f00000000000000000002","email":"E2E-Alice-Agent@e2e.test"},
			{"_id":"652f00000000000000000003","email":"e2e-alicexagent@e2e.test"}
		],"total":3,"total_available":3},"meta":{},"message":"ok"}`)
	}))
	mux.HandleFunc("POST /v2/admin/settings/t1/create-user", admin(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"error":false,"status":200,"data":{"_id":"652f0000000000000000abcd","email":"x"},"message":"ok"}`)
	}))
	mux.HandleFunc("POST /v2/admin/settings/t1/reset-password/{id}", admin(func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "boom" {
			w.WriteHeader(500)
			return
		}
		_, _ = io.WriteString(w, `{"error":false,"status":200,"data":{}}`)
	}))
	mux.HandleFunc("POST /v2/change-password", func(w http.ResponseWriter, r *http.Request) {
		f.gotAuth[r.URL.Path] = r.Header.Get("Authorization")
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		f.got[r.URL.Path] = b
		_, _ = io.WriteString(w, `{"error":false,"status":200,"data":{}}`)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return f, &HTTPApp{HubURL: ts.URL, BEURL: ts.URL, Domain: "zenifytest", TenantID: "t1", GroupID: "g1",
		AdminEmail: "admin@zenify.test", AdminPassword: adminPW}
}

func TestHTTPApp_Login(t *testing.T) {
	f, a := newUpstream(t)
	ctx := context.Background()
	if tok, err := a.Login(ctx, "top@e2e.test", userPW); err != nil || tok != "top-jwt" {
		t.Fatalf("top-level token: %q %v", tok, err)
	}
	b := f.got["/auth/login"]
	if b["domain"] != "zenifytest" || b["device_type"] != "web" || b["device_id"] != "zenify-broker" || b["email"] != "top@e2e.test" {
		t.Fatalf("login body %v", b)
	}
	if tok, err := a.Login(ctx, "nested@e2e.test", "x"); err != nil || tok != "nested-jwt" {
		t.Fatalf("data.token: %q %v", tok, err)
	}
	if _, err := a.Login(ctx, "top@e2e.test", "wrong"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("400 want ErrBadPassword, got %v", err)
	}
	_, err := a.Login(ctx, "down@e2e.test", userPW)
	if err == nil || errors.Is(err, ErrBadPassword) || !strings.Contains(err.Error(), "503") {
		t.Fatalf("503 want plain error with status, got %v", err)
	}
	if strings.Contains(err.Error(), userPW) {
		t.Fatal("error leaks the password")
	}
}

func TestHTTPApp_FindUserExactCaseInsensitive(t *testing.T) {
	f, a := newUpstream(t)
	id, found, err := a.FindUser(context.Background(), "e2e-alice-agent@e2e.test")
	if err != nil || !found || id != "652f00000000000000000002" {
		t.Fatalf("got %q %v %v", id, found, err)
	}
	if f.gotQuery != "limit=10&search=e2e-alice-agent%40e2e.test" {
		t.Fatalf("query %q", f.gotQuery)
	}
	if f.gotAuth["/v2/admin/settings/t1/list-agents"] != "Bearer admin-jwt-1" {
		t.Fatalf("auth %q", f.gotAuth)
	}
	if _, found, err := a.FindUser(context.Background(), "e2e-bob-agent@e2e.test"); err != nil || found {
		t.Fatalf("no exact match must be not found: %v %v", found, err)
	}
}

func TestHTTPApp_CreateResetChange(t *testing.T) {
	f, a := newUpstream(t)
	ctx := context.Background()
	id, err := a.CreateUser(ctx, "E2E alice agent", "e2e-alice-agent@e2e.test", "agent")
	if err != nil || id != "652f0000000000000000abcd" {
		t.Fatalf("create %q %v", id, err)
	}
	cb := f.got["/v2/admin/settings/t1/create-user"]
	if cb["name"] != "E2E alice agent" || cb["email"] != "e2e-alice-agent@e2e.test" || cb["role"] != "agent" || cb["group_id"] != "g1" {
		t.Fatalf("create body %v", cb)
	}
	if err := a.ResetPassword(ctx, id, userPW); err != nil {
		t.Fatal(err)
	}
	if rb := f.got["/v2/admin/settings/t1/reset-password/"+id]; rb["password"] != userPW {
		t.Fatalf("reset body %v", rb)
	}
	err = a.ResetPassword(ctx, "boom", userPW)
	if err == nil || !strings.Contains(err.Error(), "500") || strings.Contains(err.Error(), userPW) {
		t.Fatalf("reset failure %v", err)
	}
	if err := a.ChangePassword(ctx, "user-jwt", "old-pw", "new-pw"); err != nil {
		t.Fatal(err)
	}
	if b := f.got["/v2/change-password"]; b["password"] != "old-pw" || b["new_password"] != "new-pw" || b["c_password"] != "new-pw" {
		t.Fatalf("change body %v", b)
	}
	if f.gotAuth["/v2/change-password"] != "Bearer user-jwt" {
		t.Fatalf("change auth %q", f.gotAuth["/v2/change-password"])
	}
}

func TestHTTPApp_AdminTokenCachedAndRefreshedOn401(t *testing.T) {
	f, a := newUpstream(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, _, err := a.FindUser(ctx, "x@e2e.test"); err != nil {
			t.Fatal(err)
		}
	}
	if n := atomic.LoadInt32(&f.adminLogins); n != 1 {
		t.Fatalf("admin logged in %d times, want 1 (cached)", n)
	}
	f.adminToken.Store("admin-jwt-2") // the cached token now gets 401
	if _, _, err := a.FindUser(ctx, "x@e2e.test"); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(&f.adminLogins); n != 2 {
		t.Fatalf("admin logged in %d times, want 2 (one refresh)", n)
	}
}

func TestHTTPApp_AdminBadPasswordNoLeak(t *testing.T) {
	_, a := newUpstream(t)
	a.AdminPassword = "wrong-admin-pw"
	_, _, err := a.FindUser(context.Background(), "x@e2e.test")
	if err == nil || strings.Contains(err.Error(), "wrong-admin-pw") {
		t.Fatalf("got %v", err)
	}
}
