package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

const bootstrapMarker = "BOOTSTRAP-DIGEST-MARKER"

// seedBootstrap writes a distinctive BOOTSTRAP.txt under the temp HOME at the
// same materialized path readBootstrapDigest reads, so the tests genuinely
// exercise emission/suppression instead of passing because the file is absent.
func seedBootstrap(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "skills", "znf", "skills", "using-zenify-kit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "BOOTSTRAP.txt"), []byte(bootstrapMarker+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// FR-6.3: sentinel present in ~/.claude/CLAUDE.md => no digest emitted.
func TestSessionStart_SentinelSuppressesDigest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	seedBootstrap(t, home)
	cdir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(cdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cdir, "CLAUDE.md"),
		[]byte("# local\nznf-discipline-local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws := mkWorkspace(t)
	var buf bytes.Buffer
	code := runSessionStart(ws, &buf)
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if strings.Contains(buf.String(), bootstrapMarker) {
		t.Fatal("digest emitted despite sentinel")
	}
}

// FR-6.3: no sentinel => digest emitted raw (matching the shell hook's `cat`).
func TestSessionStart_EmitsDigestWithoutSentinel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	seedBootstrap(t, home)
	// no ~/.claude/CLAUDE.md at all => sentinelPresent() is false.
	ws := mkWorkspace(t)
	var buf bytes.Buffer
	code := runSessionStart(ws, &buf)
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(buf.String(), bootstrapMarker) {
		t.Fatalf("digest not emitted without sentinel; got %q", buf.String())
	}
}

// nudgeServer mimics github's releases/latest 302 and counts hits.
func nudgeServer(t *testing.T, tag string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Location", "/ZenifyAIContactCenter/zenify-kit/releases/tag/"+tag)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func setVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	t.Cleanup(func() { version.Version = old })
	version.Version = v
}

// SC-4: release build behind latest → exactly one nudge line, exit 0, digest still follows.
func TestSessionStart_NudgesWhenNewerRelease(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", "")
	t.Setenv("ZENIFY_NO_UPDATE_CHECK", "")
	seedBootstrap(t, home)
	srv, hits := nudgeServer(t, "v9.9.9")
	t.Setenv("ZENIFY_UPDATE_URL", srv.URL)
	setVersion(t, "0.17.3")

	var buf bytes.Buffer
	if code := runSessionStart(mkWorkspace(t), &buf); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	out := buf.String()
	want := "zenify: v9.9.9 is available (running 0.17.3) — upgrade: "
	if strings.Count(out, "is available") != 1 || !strings.Contains(out, want) {
		t.Fatalf("nudge line missing or duplicated:\n%s", out)
	}
	if !strings.Contains(out, bootstrapMarker) {
		t.Fatalf("digest must still be emitted after the nudge:\n%s", out)
	}
	if strings.Index(out, "is available") > strings.Index(out, bootstrapMarker) {
		t.Fatalf("nudge must come before the digest:\n%s", out)
	}
	if *hits != 1 {
		t.Fatalf("hits = %d, want 1", *hits)
	}
	if _, err := os.Stat(filepath.Join(home, ".zenify", "update-check.json")); err != nil {
		t.Fatalf("cache not written under HOME/.zenify: %v", err)
	}
}

// SC-4: dev build → no request, no line.
func TestSessionStart_DevBuildSkipsUpdateCheck(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", "")
	t.Setenv("ZENIFY_NO_UPDATE_CHECK", "")
	srv, hits := nudgeServer(t, "v9.9.9")
	t.Setenv("ZENIFY_UPDATE_URL", srv.URL)
	setVersion(t, "dev")

	var buf bytes.Buffer
	runSessionStart(mkWorkspace(t), &buf)
	if strings.Contains(buf.String(), "is available") || *hits != 0 {
		t.Fatalf("dev build must not check: out=%q hits=%d", buf.String(), *hits)
	}
}

// SC-4: ZENIFY_NO_UPDATE_CHECK set → same as dev.
func TestSessionStart_NoUpdateCheckEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", "")
	t.Setenv("ZENIFY_NO_UPDATE_CHECK", "1")
	srv, hits := nudgeServer(t, "v9.9.9")
	t.Setenv("ZENIFY_UPDATE_URL", srv.URL)
	setVersion(t, "0.17.3")

	var buf bytes.Buffer
	runSessionStart(mkWorkspace(t), &buf)
	if strings.Contains(buf.String(), "is available") || *hits != 0 {
		t.Fatalf("opt-out must not check: out=%q hits=%d", buf.String(), *hits)
	}
}

// Up to date → silent.
func TestSessionStart_UpToDateIsSilent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", "")
	t.Setenv("ZENIFY_NO_UPDATE_CHECK", "")
	srv, _ := nudgeServer(t, "v0.17.3")
	t.Setenv("ZENIFY_UPDATE_URL", srv.URL)
	setVersion(t, "0.17.3")

	var buf bytes.Buffer
	runSessionStart(mkWorkspace(t), &buf)
	if strings.Contains(buf.String(), "is available") {
		t.Fatalf("equal versions must not nudge: %q", buf.String())
	}
}

// Network failure → silent, exit 0 (fail-open).
func TestSessionStart_UpdateFailureIsSilent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", "")
	t.Setenv("ZENIFY_NO_UPDATE_CHECK", "")
	srv, _ := nudgeServer(t, "v9.9.9")
	url := srv.URL
	srv.Close()
	t.Setenv("ZENIFY_UPDATE_URL", url)
	setVersion(t, "0.17.3")

	var buf bytes.Buffer
	if code := runSessionStart(mkWorkspace(t), &buf); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if strings.Contains(buf.String(), "is available") || strings.Contains(buf.String(), "update") {
		t.Fatalf("failure must be silent on stdout: %q", buf.String())
	}
}

// writeBrokerCache writes ~/.zenify/broker/token.json under home.
func writeBrokerCache(t *testing.T, home, body string) {
	t.Helper()
	dir := filepath.Join(home, ".zenify", "broker")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A dev session learns its prd limits from the cache alone (no network, an
// expired token still counts); lead, role-less and absent caches print nothing.
func TestSessionStart_BrokerRoleLine(t *testing.T) {
	t.Setenv("ZENIFY_HOME", "")
	t.Setenv("ZENIFY_NO_UPDATE_CHECK", "1")
	devLine := "broker: login=devgal role=dev — prd: " + api.DevPrdSummary() + "; cần dữ liệu prd thì dùng stg hoặc nhờ lead\n"                                           //znf:allow-lang
	dataLine := `data: agent tự chạy zenify data run --stg|--prd [--write] -- <lệnh> (dry-run trước; --prd --write chờ "ok" của người dùng; xem _shared/data-scripts.md)` //znf:allow-lang
	agentLine := "broker: login=bot role=agent — prd: read only; ghi prd cần lead, dừng bằng zenify await data-write\n" + dataLine + "\n"                                 //znf:allow-lang
	for name, c := range map[string]struct{ cache, want string }{
		"dev expired": {`{"url":"https://b.example","token":"jwt-secret-value","expires_at":"2000-01-01T00:00:00Z","login":"devgal","role":"dev"}`, devLine + dataLine + "; prd: cần lead\n"}, //znf:allow-lang
		"lead":        {`{"url":"https://b.example","token":"jwt-secret-value","expires_at":"2099-01-01T00:00:00Z","login":"boss","role":"lead"}`, dataLine + "\n"},
		"agent":       {`{"url":"https://b.example","token":"jwt-secret-value","expires_at":"2099-01-01T00:00:00Z","login":"bot","role":"agent"}`, agentLine},
		"no role":     {`{"url":"https://b.example","token":"jwt-secret-value","expires_at":"2099-01-01T00:00:00Z","login":"old"}`, dataLine + "\n"},
		"no cache":    {"", ""},
	} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if c.cache != "" {
			writeBrokerCache(t, home, c.cache)
		}
		var line bytes.Buffer
		brokerRoleLine(&line)
		if line.String() != c.want {
			t.Errorf("%s: line = %q, want %q", name, line.String(), c.want)
		}
		var buf bytes.Buffer
		runSessionStart(mkWorkspace(t), &buf)
		if strings.Contains(buf.String(), "jwt-secret-value") || (c.want != "" && !strings.Contains(buf.String(), c.want)) ||
			(c.want == "" && strings.Contains(buf.String(), "data: zenify data run")) {
			t.Errorf("%s: session-start output = %q", name, buf.String())
		}
	}
}

// FR-5.3: ZNF_AUTONOMOUS=1 (exactly "1") makes SessionStart point at the autonomous doc,
// even when the bootstrap digest is printed.
func TestSessionStart_AutonomousLine(t *testing.T) {
	t.Setenv("ZENIFY_HOME", "")
	t.Setenv("ZENIFY_NO_UPDATE_CHECK", "1")
	want := "autonomous: ZNF_AUTONOMOUS=1 — read ~/.claude/skills/znf/skills/_shared/autonomous.md first; it overrides every human gate in znf skills\n"
	for _, v := range []string{"1", "0", ""} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		digest := filepath.Join(home, ".claude", "skills", "znf", "skills", "using-zenify-kit")
		if err := os.MkdirAll(digest, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(digest, "BOOTSTRAP.txt"), []byte("DIGEST\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("ZNF_AUTONOMOUS", v)
		var buf bytes.Buffer
		runSessionStart(mkWorkspace(t), &buf)
		out := buf.String()
		if !strings.Contains(out, "DIGEST") {
			t.Fatalf("ZNF_AUTONOMOUS=%q: digest not printed: %q", v, out)
		}
		if got := strings.Contains(out, "_shared/autonomous.md"); got != (v == "1") {
			t.Errorf("ZNF_AUTONOMOUS=%q: output = %q", v, out)
		}
		if v == "1" && !strings.Contains(out, want) {
			t.Errorf("ZNF_AUTONOMOUS=1: missing line %q in %q", want, out)
		}
	}
}

func TestSessionStart_FeaturesLine(t *testing.T) {
	t.Setenv("ZENIFY_HOME", "")
	t.Setenv("ZENIFY_NO_UPDATE_CHECK", "1")
	t.Setenv("ZNF_AUTONOMOUS", "")
	want := "features: github.issues on — znf:ship links a GitHub issue (Closes #N) before opening a PR; znf:nonstop plans from assigned issues\n"
	cases := []struct {
		name    string
		overlay *string
		wantOn  bool
	}{
		{"on", ptr("features:\n  github.issues: true\n"), true},
		{"off", ptr("features:\n  github.issues: false\n"), false},
		{"no overlay", nil, false},
		{"broken overlay", ptr("repos: [\n"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			ws := mkWorkspace(t)
			if c.overlay != nil {
				if err := os.WriteFile(filepath.Join(ws, ".zenify-overlay.yaml"), []byte(*c.overlay), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var buf bytes.Buffer
			if rc := runSessionStart(ws, &buf); rc != 0 {
				t.Fatalf("rc = %d", rc)
			}
			out := buf.String()
			if c.wantOn && !strings.Contains(out, want) {
				t.Errorf("missing line %q in %q", want, out)
			}
			if !c.wantOn && strings.Contains(out, "features:") {
				t.Errorf("unexpected features line in %q", out)
			}
		})
	}
}

func ptr(s string) *string { return &s }
