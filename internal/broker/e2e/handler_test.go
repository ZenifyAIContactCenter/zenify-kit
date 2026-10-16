package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
)

var jwtKey = []byte("jwt-key-0123456789abcdef01234567")

func serveE2E(t *testing.T, app App, body string) (int, []byte, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	ts := httptest.NewServer(auth.Require(jwtKey, &Handler{P: newProvisioner(app), Audit: &audit.Logger{W: buf}}))
	t.Cleanup(ts.Close)
	tok, _, _ := auth.Issue(jwtKey, "alice", "lead", time.Now())
	r, _ := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, buf.String()
}

func TestHandler_UnknownRole(t *testing.T) {
	for _, role := range []string{"end_user", "chatbot", "", "Agent"} {
		app := newFakeApp()
		st, body, _ := serveE2E(t, app, `{"role":"`+role+`"}`)
		var e api.Error
		_ = json.Unmarshal(body, &e)
		if st != 400 || e.Error != api.CodeUnknownRole {
			t.Fatalf("role %q: got %d %s", role, st, body)
		}
		if app.logins != 0 {
			t.Fatalf("role %q reached the app", role)
		}
	}
}

func TestHandler_InvalidBody(t *testing.T) {
	st, body, _ := serveE2E(t, newFakeApp(), `{"role":`)
	var e api.Error
	_ = json.Unmarshal(body, &e)
	if st != 400 || e.Error != api.CodeInvalidBody {
		t.Fatalf("got %d %s", st, body)
	}
}

func TestHandler_AuditNoPassword(t *testing.T) {
	st, body, line := serveE2E(t, newFakeApp(), `{"role":"agent"}`)
	if st != 200 {
		t.Fatalf("got %d %s", st, body)
	}
	var resp api.E2EAccountResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Email != aliceAgent || resp.Password != Password([]byte("testkey"), "alice", "agent", 230) || resp.Role != "agent" || resp.Domain != "zenifytest" {
		t.Fatalf("resp %+v", resp)
	}
	lines := strings.Split(strings.TrimSpace(line), "\n")
	if len(lines) != 1 {
		t.Fatalf("want 1 audit line, got %q", line)
	}
	var e audit.Entry
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatal(err)
	}
	if e.Op != "agent" || e.Login != "alice" || e.Endpoint != "/e2e/account" || e.Env != "stg" || e.Status != 200 {
		t.Fatalf("entry %+v", e)
	}
	for _, secret := range []string{resp.Password, resp.Email, "tok:"} {
		if strings.Contains(lines[0], secret) {
			t.Fatalf("audit line leaks %q: %s", secret, lines[0])
		}
	}
}

func TestHandler_ProvisionFailed502(t *testing.T) {
	app := newFakeApp()
	app.loginErr = errors.New("hub login: status 503")
	st, body, line := serveE2E(t, app, `{"role":"sale"}`)
	var e api.Error
	_ = json.Unmarshal(body, &e)
	if st != 502 || e.Error != api.CodeProvisionFailed {
		t.Fatalf("got %d %s", st, body)
	}
	if !strings.Contains(line, `"status":502`) || !strings.Contains(line, `"op":"sale"`) {
		t.Fatalf("audit %s", line)
	}
}

func TestHandler_MFARequired502(t *testing.T) {
	app := newFakeApp()
	app.loginErr = ErrMFARequired
	st, body, _ := serveE2E(t, app, `{"role":"agent"}`)
	var e api.Error
	_ = json.Unmarshal(body, &e)
	if st != 502 || e.Error != api.CodeProvisionFailed || !strings.Contains(e.Message, "MFA") {
		t.Fatalf("got %d %s", st, body)
	}
}

func TestHandler_Validate(t *testing.T) {
	ok := &Handler{Audit: &audit.Logger{W: io.Discard},
		P: &Provisioner{App: newFakeApp(), Key: []byte(strings.Repeat("k", 32)), Now: time.Now}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid handler: %v", err)
	}
	if err := (&Handler{}).Validate(); err == nil || !strings.Contains(err.Error(), "Audit is nil") ||
		!strings.Contains(err.Error(), "Provisioner is nil") {
		t.Errorf("Validate() = %v", err)
	}
	short := &Handler{Audit: ok.Audit, P: &Provisioner{App: newFakeApp(), Key: []byte("short"), Now: time.Now}}
	if err := short.Validate(); err == nil || !strings.Contains(err.Error(), "Key") {
		t.Errorf("short key: Validate() = %v", err)
	}
}
