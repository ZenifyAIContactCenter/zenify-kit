package creds

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func fixture() *Handler {
	return &Handler{
		Read: map[string]map[string]string{
			"stg": {"MONGO_URL": "mongodb://ro-stg-secret", "MYSQL_URL": "mysql://ro-stg-secret", "REDIS_URL": "redis://ro-stg-secret"},
			"prd": {"MONGO_URL": "mongodb://ro-prd-secret"},
		},
		Write: map[string]map[string]string{
			"stg": {"MONGO_URL": "mongodb://rw-stg-secret", "MYSQL_URL": "mysql://rw-stg-secret"},
		},
		Lumi: map[string]map[string]string{
			"stg": {"LUMI_MYSQL_URL": "mysql://lumi-stg-secret", "QDRANT_URL": "http://qdrant-stg-secret", "LUMI_APP_URL": "http://lumi-app-stg"},
			"prd": {"LUMI_MYSQL_URL": "mysql://lumi-prd-secret"},
		},
	}
}

func serve(t *testing.T, h *Handler, role, body string) (int, []byte, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	h.Audit = &audit.Logger{W: buf}
	ts := httptest.NewServer(auth.Require(jwtKey, h))
	t.Cleanup(ts.Close)
	tok, _, _ := auth.Issue(jwtKey, "alice", role, time.Now())
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

func values(t *testing.T, b []byte) map[string]string {
	t.Helper()
	var r api.CredsResponse
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("%s: %v", b, err)
	}
	return r.Values
}

func errCode(b []byte) (string, string) {
	var e api.Error
	_ = json.Unmarshal(b, &e)
	return e.Error, e.Message
}

func TestCreds_DevPrdLeadOnly(t *testing.T) {
	st, body, _ := serve(t, fixture(), "dev", `{"env":"prd","mode":"read"}`)
	if c, _ := errCode(body); st != 403 || c != api.CodeLeadOnly {
		t.Fatalf("got %d %s", st, body)
	}
}

func TestCreds_LeadPrdRead(t *testing.T) {
	st, body, _ := serve(t, fixture(), "lead", `{"env":"prd","mode":"read"}`)
	if st != 200 || values(t, body)["MONGO_URL"] != "mongodb://ro-prd-secret" {
		t.Fatalf("got %d %s", st, body)
	}
}

func TestCreds_DevStgWrite(t *testing.T) {
	st, body, _ := serve(t, fixture(), "dev", `{"env":"stg","mode":"write"}`)
	if st != 200 || values(t, body)["MONGO_URL"] != "mongodb://rw-stg-secret" {
		t.Fatalf("got %d %s", st, body)
	}
	if _, ok := values(t, body)["REDIS_URL"]; ok {
		t.Fatalf("engine without a secret must be absent: %s", body)
	}
}

func TestCreds_ReadUsesReadSet(t *testing.T) {
	st, body, _ := serve(t, fixture(), "dev", `{"env":"stg","mode":"read"}`)
	if st != 200 || values(t, body)["MONGO_URL"] != "mongodb://ro-stg-secret" {
		t.Fatalf("got %d %s", st, body)
	}
}

func TestCreds_WriteMissing(t *testing.T) {
	st, body, _ := serve(t, fixture(), "lead", `{"env":"prd","mode":"write"}`)
	c, msg := errCode(body)
	if st != 500 || c != api.CodeBrokerMisconfigured || !strings.Contains(msg, "prd") {
		t.Fatalf("got %d %s", st, body)
	}
	if strings.Contains(string(body), "secret") {
		t.Fatalf("leaks a value: %s", body)
	}
}

func TestCreds_LumiOptIn(t *testing.T) {
	_, body, _ := serve(t, fixture(), "dev", `{"env":"stg","mode":"read"}`)
	v := values(t, body)
	for _, k := range []string{"LUMI_MYSQL_URL", "QDRANT_URL", "LUMI_APP_URL"} {
		if _, ok := v[k]; ok {
			t.Fatalf("%s present without lumi: %s", k, body)
		}
	}
	_, body, _ = serve(t, fixture(), "dev", `{"env":"stg","mode":"read","lumi":true}`)
	v = values(t, body)
	if v["LUMI_MYSQL_URL"] != "mysql://lumi-stg-secret" || v["QDRANT_URL"] != "http://qdrant-stg-secret" || v["LUMI_APP_URL"] != "http://lumi-app-stg" {
		t.Fatalf("lumi values: %s", body)
	}
}

func TestCreds_BadBody(t *testing.T) {
	for _, b := range []string{`{"env":"x","mode":"read"}`, `{"env":"stg","mode":"y"}`, `{"env":`} {
		st, body, _ := serve(t, fixture(), "dev", b)
		if c, _ := errCode(body); st != 400 || c != api.CodeInvalidBody {
			t.Fatalf("%s: got %d %s", b, st, body)
		}
	}
}

func TestCreds_AuditNoValues(t *testing.T) {
	st, _, line := serve(t, fixture(), "dev", `{"env":"stg","mode":"write","lumi":true}`)
	if st != 200 {
		t.Fatalf("got %d", st)
	}
	lines := strings.Split(strings.TrimSpace(line), "\n")
	if len(lines) != 1 {
		t.Fatalf("want 1 audit line, got %q", line)
	}
	var e audit.Entry
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatal(err)
	}
	if e.Endpoint != "/creds" || e.Env != "stg" || e.Op != "write" || e.Login != "alice" || e.Role != "dev" || e.Status != 200 {
		t.Fatalf("entry %+v", e)
	}
	if strings.Contains(line, "secret") || strings.Contains(line, "://") {
		t.Fatalf("audit line leaks a value: %s", line)
	}
}

func TestCreds_AuditOnRefusal(t *testing.T) {
	_, _, line := serve(t, fixture(), "dev", `{"env":"prd","mode":"read"}`)
	var e audit.Entry
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &e); err != nil || e.Status != 403 || e.Env != "prd" || e.Op != "read" {
		t.Fatalf("audit %s", line)
	}
}

func TestValidate(t *testing.T) {
	if err := (&Handler{}).Validate(); err == nil {
		t.Fatal("nil Audit must fail Validate")
	}
	if err := (&Handler{Audit: &audit.Logger{W: &bytes.Buffer{}}}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCreds_RoleEnvMode(t *testing.T) {
	for _, c := range []struct {
		role, env, mode string
		lumi            bool
		want            int
	}{
		{"agent", "stg", "read", false, 200}, {"agent", "stg", "write", false, 200},
		{"agent", "prd", "read", false, 200}, {"agent", "prd", "write", false, 403},
		// lumi-agent's prd credentials are its own read-write ones: lead only.
		{"agent", "stg", "read", true, 200}, {"agent", "prd", "read", true, 403},
		{"dev", "prd", "read", false, 403}, {"dev", "prd", "write", false, 403},
		{"lead", "stg", "read", false, 200}, {"lead", "stg", "write", false, 200},
		{"lead", "prd", "read", false, 200}, {"lead", "prd", "write", false, 200},
		{"lead", "prd", "read", true, 200},
	} {
		h := fixture()
		h.Write["prd"] = map[string]string{"MONGO_URL": "mongodb://rw-prd-secret"}
		st, body, _ := serve(t, h, c.role, fmt.Sprintf(`{"env":%q,"mode":%q,"lumi":%t}`, c.env, c.mode, c.lumi))
		if st != c.want {
			t.Errorf("%s %s/%s lumi=%t: got %d %s, want %d", c.role, c.env, c.mode, c.lumi, st, body, c.want)
		}
		if c.want == 403 {
			if code, _ := errCode(body); code != api.CodeLeadOnly {
				t.Errorf("%s %s/%s lumi=%t: code %q", c.role, c.env, c.mode, c.lumi, code)
			}
			if strings.Contains(string(body), "rw-prd-secret") || strings.Contains(string(body), "lumi-prd-secret") {
				t.Errorf("%s: leaked prd creds", c.role)
			}
		}
	}
}

func TestCreds_AuditRecordsTask(t *testing.T) {
	h := fixture()
	buf := &bytes.Buffer{}
	h.Audit = &audit.Logger{W: buf}
	ts := httptest.NewServer(auth.Require(jwtKey, h))
	defer ts.Close()
	tok, _, _ := auth.Issue(jwtKey, "alice", "agent", time.Now())
	r, _ := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(`{"env":"stg","mode":"read"}`))
	r.Header.Set("Authorization", "Bearer "+tok)
	r.Header.Set("X-Zenify-Task", "ZEN-12")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if !strings.Contains(buf.String(), `"task":"ZEN-12"`) {
		t.Errorf("audit = %s", buf.String())
	}
}
