package mcpproxy_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/mcpproxy"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/server"
)

var jwtKey = []byte("jwt-test-key-0123456789abcdef0123")

// stack is the full broker (server.New + auth.Require) with one MCP route.
func stack(t *testing.T, name string, upstream string, set func(http.Header), out *bytes.Buffer) (*httptest.Server, string) {
	t.Helper()
	u, err := url.Parse(upstream)
	if err != nil {
		t.Fatal(err)
	}
	lg := &audit.Logger{W: out}
	h := server.New(server.Deps{JWTKey: jwtKey, Audit: lg,
		MCP: map[string]http.Handler{name: mcpproxy.New(name, u, set, lg)}})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	tok, _, _ := auth.Issue(jwtKey, "octocat", time.Now())
	return srv, tok
}

func do(t *testing.T, method, u, tok, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, u, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func auditLines(t *testing.T, out *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("audit line %q: %v", l, err)
		}
		lines = append(lines, m)
	}
	return lines
}

func errBody(t *testing.T, resp *http.Response) (string, string) {
	t.Helper()
	var e struct{ Error, Message string }
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		t.Fatal(err)
	}
	return e.Error, e.Message
}

// SC-6: the client's broker JWT and cookies never reach upstream; the
// upstream credential and read-only header do; MCP headers pass intact.
func TestProxy_StripsClientAuth(t *testing.T) {
	got := make(chan *http.Request, 1)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Clone(context.Background())
		w.Header().Set("Mcp-Session-Id", "sess-2")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer up.Close()
	var out bytes.Buffer
	srv, tok := stack(t, "posthog", up.URL+"/mcp", mcpproxy.PostHogHeaders("test-key", "42"), &out)

	resp := do(t, http.MethodPost, srv.URL+"/mcp/posthog/extra?x=1", tok, `{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		map[string]string{"Cookie": "c=1", "Mcp-Session-Id": "sess-1", "Accept": "application/json, text/event-stream",
			"Content-Type": "application/json", "MCP-Protocol-Version": "2025-06-18", "Last-Event-ID": "ev-7"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if resp.Header.Get("Mcp-Session-Id") != "sess-2" {
		t.Errorf("response Mcp-Session-Id = %q", resp.Header.Get("Mcp-Session-Id"))
	}
	r := <-got
	h := r.Header
	if a := h.Values("Authorization"); len(a) != 1 || a[0] != "Bearer test-key" {
		t.Errorf("upstream Authorization = %q", a)
	}
	for k, vs := range h {
		for _, v := range vs {
			if strings.Contains(v, tok) {
				t.Errorf("client JWT reached upstream in %s", k)
			}
		}
	}
	if h.Get("Cookie") != "" {
		t.Errorf("Cookie reached upstream: %q", h.Get("Cookie"))
	}
	want := map[string]string{"x-posthog-read-only": "true", "x-posthog-project-id": "42", "Mcp-Session-Id": "sess-1",
		"Accept": "application/json, text/event-stream", "Content-Type": "application/json",
		"MCP-Protocol-Version": "2025-06-18", "Last-Event-ID": "ev-7"}
	for k, v := range want {
		if h.Get(k) != v {
			t.Errorf("upstream %s = %q, want %q", k, h.Get(k), v)
		}
	}
	upHost := strings.TrimPrefix(up.URL, "http://")
	if r.Host != upHost {
		t.Errorf("upstream Host = %q, want %q", r.Host, upHost)
	}
	if r.URL.Path != "/mcp" || r.URL.RawQuery != "x=1" {
		t.Errorf("upstream URL = %s?%s, want /mcp?x=1", r.URL.Path, r.URL.RawQuery)
	}
}

// SSE goes through the full stack unbuffered: the first event reaches the
// client while the upstream still holds the stream open.
func TestProxy_StreamsSSE(t *testing.T) {
	release := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: 1\n\n"))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer up.Close()
	defer close(release)
	var out bytes.Buffer
	srv, tok := stack(t, "grafana", up.URL+"/mcp", nil, &out)

	// The request runs in the goroutine too: unflushed headers would block
	// Do itself, and the timeout below must cover that as well.
	line := make(chan string, 1)
	go func() {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/mcp/grafana", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			line <- err.Error()
			return
		}
		defer func() { _ = resp.Body.Close() }()
		s, _ := bufio.NewReader(resp.Body).ReadString('\n')
		line <- s
	}()
	select {
	case s := <-line:
		if s != "data: 1\n" {
			t.Fatalf("first line %q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first SSE event did not arrive before upstream closed")
	}
}

// FR-4.4 + FR-6.1: two tools/call in a batch make two lines, plus exactly one
// request-level line.
func TestProxy_AuditToolsCall(t *testing.T) {
	const respBody = `[{"jsonrpc":"2.0","id":1,"result":{}}]`
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(respBody))
	}))
	defer up.Close()
	var out bytes.Buffer
	srv, tok := stack(t, "grafana", up.URL+"/mcp", nil, &out)

	body := `[{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"query_prometheus","arguments":{"expr":"up"}}},
	          {"jsonrpc":"2.0","id":2,"method":"initialize","params":{}},
	          {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_datasources","arguments":{}}}]`
	resp := do(t, http.MethodPost, srv.URL+"/mcp/grafana", tok, body, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	_, _ = bufio.NewReader(resp.Body).ReadString(0)
	srv.Close() // wait for the handler (and its deferred audit) to finish

	lines := auditLines(t, &out)
	if len(lines) != 3 {
		t.Fatalf("got %d audit lines, want 3:\n%s", len(lines), out.String())
	}
	var ops []string
	for _, l := range lines {
		ops = append(ops, l["op"].(string))
		if l["endpoint"] != "/mcp/grafana" || l["login"] != "octocat" {
			t.Errorf("line %v: endpoint/login", l)
		}
	}
	slices.Sort(ops)
	if !slices.Equal(ops, []string{"POST", "list_datasources", "query_prometheus"}) {
		t.Errorf("ops = %v", ops)
	}
	for _, l := range lines {
		switch l["op"] {
		case "query_prometheus":
			if a, _ := l["args"].(map[string]any); a["expr"] != "up" {
				t.Errorf("args = %v", l["args"])
			}
		case "POST":
			if l["status"] != float64(200) || l["bytes"] != float64(len(respBody)) {
				t.Errorf("request line status/bytes = %v/%v, want 200/%d", l["status"], l["bytes"], len(respBody))
			}
		}
	}
}

func TestProxy_UnparseableBodyForwardedOneLine(t *testing.T) {
	got := make(chan string, 1)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := new(bytes.Buffer)
		_, _ = b.ReadFrom(r.Body)
		got <- b.String()
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer up.Close()
	var out bytes.Buffer
	srv, tok := stack(t, "grafana", up.URL+"/mcp", nil, &out)
	resp := do(t, http.MethodPost, srv.URL+"/mcp/grafana", tok, `{not json`, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want upstream's 400", resp.StatusCode)
	}
	if b := <-got; b != `{not json` {
		t.Errorf("upstream body %q", b)
	}
	srv.Close()
	lines := auditLines(t, &out)
	if len(lines) != 1 || lines[0]["op"] != "POST" || lines[0]["status"] != float64(400) {
		t.Errorf("audit = %v", lines)
	}
}

func TestProxy_BodyTooLarge413(t *testing.T) {
	hit := make(chan struct{}, 1)
	up := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit <- struct{}{} }))
	defer up.Close()
	var out bytes.Buffer
	srv, tok := stack(t, "grafana", up.URL+"/mcp", nil, &out)
	resp := do(t, http.MethodPost, srv.URL+"/mcp/grafana", tok, strings.Repeat("a", 4<<20+1), nil)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if code, _ := errBody(t, resp); code != "body_too_large" {
		t.Errorf("code %q", code)
	}
	select {
	case <-hit:
		t.Error("oversized body reached upstream")
	default:
	}
}

func TestProxy_UpstreamDown502(t *testing.T) {
	up := httptest.NewServer(http.NotFoundHandler())
	upURL := up.URL
	up.Close()
	var out bytes.Buffer
	srv, tok := stack(t, "grafana", upURL+"/mcp", nil, &out)
	resp := do(t, http.MethodPost, srv.URL+"/mcp/grafana", tok, `{}`, nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status %d", resp.StatusCode)
	}
	code, msg := errBody(t, resp)
	if code != "upstream_unavailable" {
		t.Errorf("code %q", code)
	}
	if strings.Contains(msg, strings.TrimPrefix(upURL, "http://")) {
		t.Errorf("message leaks upstream address: %q", msg)
	}
	srv.Close()
	if lines := auditLines(t, &out); len(lines) != 1 || lines[0]["status"] != float64(502) {
		t.Errorf("audit = %v", lines)
	}
}

func TestMCPRoute_AuthAndUnknownName(t *testing.T) {
	up := httptest.NewServer(http.NotFoundHandler())
	defer up.Close()
	var out bytes.Buffer
	srv, tok := stack(t, "grafana", up.URL+"/mcp", nil, &out)
	if resp := do(t, http.MethodPost, srv.URL+"/mcp/grafana", "", `{}`, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no JWT: status %d", resp.StatusCode)
	}
	if resp := do(t, http.MethodPost, srv.URL+"/mcp/nope", tok, `{}`, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown name: status %d", resp.StatusCode)
	}
	if resp := do(t, http.MethodDelete, srv.URL+"/mcp/grafana", tok, "", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("DELETE should reach upstream (404 from it): status %d", resp.StatusCode)
	}
}

func TestStartGrafana_Args(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "out")
	bin := filepath.Join(dir, "fake-mcp-grafana")
	script := "#!/bin/sh\nfor a in \"$@\"; do echo \"arg:$a\"; done > " + outFile + "\n" +
		"[ -n \"$BROKER_SECRET\" ] && echo leaked >> " + outFile + "\n" +
		"echo \"url:$GRAFANA_URL\" >> " + outFile + "\necho \"tok:$GRAFANA_SERVICE_ACCOUNT_TOKEN\" >> " + outFile + "\nexit 3\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BROKER_SECRET", "must-not-leak")

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	c, err := mcpproxy.StartGrafana(ctx, cancel, bin, "http://grafana.test", "test-key", "127.0.0.1:8001",
		[]string{"--loki-guardrail-mode=enforce", "--usage-stats=disabled"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("child exit did not cancel the server context")
	}
	if cause := context.Cause(ctx); cause == nil || errors.Is(cause, context.Canceled) {
		t.Errorf("cause = %v, want the child's exit error", cause)
	}

	wantArgs := []string{bin, "-t", "streamable-http", "--address", "127.0.0.1:8001", "--disable-write",
		"--loki-guardrail-mode=enforce", "--usage-stats=disabled"}
	if !slices.Equal(c.Args, wantArgs) {
		t.Errorf("argv = %q, want %q", c.Args, wantArgs)
	}
	var keys []string
	for _, kv := range c.Env {
		k, _, _ := strings.Cut(kv, "=")
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, []string{"GRAFANA_SERVICE_ACCOUNT_TOKEN", "GRAFANA_URL", "HOME", "PATH"}) {
		t.Errorf("env keys = %v", keys)
	}
	b, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	want := "arg:-t\narg:streamable-http\narg:--address\narg:127.0.0.1:8001\narg:--disable-write\narg:--loki-guardrail-mode=enforce\narg:--usage-stats=disabled\nurl:http://grafana.test\ntok:test-key\n"
	if string(b) != want {
		t.Errorf("child saw:\n%s\nwant:\n%s", b, want)
	}
}

// With no upstream credential (grafana's child needs none on the wire) the
// client's Authorization is still dropped, not passed through.
func TestProxy_StripsClientAuthWithoutSetHeaders(t *testing.T) {
	got := make(chan http.Header, 1)
	up := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got <- r.Header.Clone() }))
	defer up.Close()
	var out bytes.Buffer
	srv, tok := stack(t, "grafana", up.URL+"/mcp", nil, &out)
	do(t, http.MethodPost, srv.URL+"/mcp/grafana", tok, `{}`, nil)
	if a := (<-got).Values("Authorization"); len(a) != 0 {
		t.Errorf("client Authorization reached upstream: %d value(s)", len(a))
	}
}

// toolOps posts body through the stack and returns the ops of the tools/call
// audit lines (the request-level POST line removed).
func toolOps(t *testing.T, body string) []string {
	t.Helper()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer up.Close()
	var out bytes.Buffer
	srv, tok := stack(t, "grafana", up.URL+"/mcp", nil, &out)
	do(t, http.MethodPost, srv.URL+"/mcp/grafana", tok, body, nil)
	srv.Close()
	var ops []string
	for _, l := range auditLines(t, &out) {
		if l["op"] != "POST" {
			ops = append(ops, l["op"].(string))
		}
	}
	return ops
}

// A junk element in a batch hides only itself, not the tools/call next to it.
func TestProxy_AuditBatchWithJunkElements(t *testing.T) {
	body := `[1,"x",{"method":5},null,{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"q","arguments":{"a":1}}}]`
	if ops := toolOps(t, body); !slices.Equal(ops, []string{"q"}) {
		t.Errorf("ops = %v, want [q]", ops)
	}
}

// Keys match exactly, as a JS upstream reads them: "Method" does not shadow
// "method", and "Method" alone is no tools/call.
func TestProxy_AuditExactKeys(t *testing.T) {
	if ops := toolOps(t, `{"method":"tools/call","Method":"ping","params":{"name":"q","Name":"other"}}`); !slices.Equal(ops, []string{"q"}) {
		t.Errorf("ops = %v, want [q]", ops)
	}
	if ops := toolOps(t, `{"Method":"tools/call","params":{"name":"q"}}`); len(ops) != 0 {
		t.Errorf("ops = %v, want none", ops)
	}
}

// A tools/call with malformed or nameless params is audited as op "invalid".
func TestProxy_AuditMalformedParamsAsInvalid(t *testing.T) {
	for _, body := range []string{
		`{"method":"tools/call","params":"junk"}`,
		`{"method":"tools/call"}`,
		`{"method":"tools/call","params":{"name":5}}`,
	} {
		if ops := toolOps(t, body); !slices.Equal(ops, []string{"invalid"}) {
			t.Errorf("%s: ops = %v, want [invalid]", body, ops)
		}
	}
}

func TestProxy_StripsEdgeHeaders(t *testing.T) {
	got := make(chan http.Header, 1)
	up := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got <- r.Header.Clone() }))
	defer up.Close()
	var out bytes.Buffer
	srv, tok := stack(t, "grafana", up.URL+"/mcp", nil, &out)
	do(t, http.MethodPost, srv.URL+"/mcp/grafana", tok, `{}`, map[string]string{
		"CF-Connecting-IP": "203.0.113.9", "Cf-Ray": "r1", "Cf-Access-Jwt-Assertion": "a", "X-Real-IP": "203.0.113.9",
		"X-Forwarded-For": "203.0.113.9", "X-Forwarded-Host": "h", "X-Forwarded-Proto": "https", "Forwarded": "for=x",
		"X-Grafana-Org-Id": "2", "X-Grafana-User": "admin"})
	h := <-got
	for k := range h {
		ck := http.CanonicalHeaderKey(k)
		if strings.HasPrefix(ck, "Cf-") || strings.HasPrefix(ck, "X-Forwarded-") || strings.HasPrefix(ck, "X-Grafana-") ||
			ck == "X-Real-Ip" || ck == "Forwarded" {
			t.Errorf("%s reached upstream", ck)
		}
	}
}

// FR-6.1: an authenticated request to an unknown name is audited too.
func TestMCPRoute_UnknownNameAudited(t *testing.T) {
	up := httptest.NewServer(http.NotFoundHandler())
	defer up.Close()
	var out bytes.Buffer
	srv, tok := stack(t, "grafana", up.URL+"/mcp", nil, &out)
	if resp := do(t, http.MethodPost, srv.URL+"/mcp/nope", tok, `{}`, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
	srv.Close()
	lines := auditLines(t, &out)
	if len(lines) != 1 || lines[0]["status"] != float64(404) || lines[0]["endpoint"] != "/mcp/nope" ||
		lines[0]["login"] != "octocat" || lines[0]["op"] != "POST" {
		t.Errorf("audit = %v", lines)
	}
}

// The client hangs up while the upstream is still working: audited as 499.
func TestProxy_ClientGoneAudited499(t *testing.T) {
	hit := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body) // lets the server notice the hang-up
		close(hit)
		<-r.Context().Done()
	}))
	defer up.Close()
	var out bytes.Buffer
	srv, tok := stack(t, "grafana", up.URL+"/mcp", nil, &out)
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/mcp/grafana", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	done := make(chan struct{})
	go func() {
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
		}
		close(done)
	}()
	<-hit
	cancel()
	<-done
	srv.Close()
	lines := auditLines(t, &out)
	if len(lines) != 1 || lines[0]["status"] != float64(499) {
		t.Errorf("audit = %v", lines)
	}
}

func TestNew_NilAuditLogger(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer up.Close()
	u, _ := url.Parse(up.URL + "/mcp")
	rec := httptest.NewRecorder()
	mcpproxy.New("grafana", u, nil, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp/grafana", strings.NewReader(`{}`)))
	if rec.Code != http.StatusOK {
		t.Errorf("status %d", rec.Code)
	}
}

func TestStartGrafana_RefusesNonLoopbackAddr(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:8000", ":8000", "10.1.2.3:8000", "[::]:8000", "grafana.internal:8000", "127.0.0.1"} {
		ctx, cancel := context.WithCancelCause(context.Background())
		_, err := mcpproxy.StartGrafana(ctx, cancel, "/nonexistent/mcp-grafana", "http://grafana.test", "test-key", addr, nil)
		cancel(nil)
		if err == nil || !strings.Contains(err.Error(), "loopback") || strings.Contains(err.Error(), "test-key") {
			t.Errorf("%q: err = %v", addr, err)
		}
	}
	for _, addr := range []string{"127.0.0.1:8000", "[::1]:8000", "localhost:8000"} {
		ctx, cancel := context.WithCancelCause(context.Background())
		_, err := mcpproxy.StartGrafana(ctx, cancel, "/nonexistent/mcp-grafana", "http://grafana.test", "test-key", addr, nil)
		cancel(nil)
		if err == nil || strings.Contains(err.Error(), "loopback") {
			t.Errorf("%q: want only the start failure, got %v", addr, err)
		}
	}
}

func TestStartGrafana_RefusesFixedFlagsInExtraArgs(t *testing.T) {
	for _, a := range []string{"--disable-write=false", "-disable-write", "--address=0.0.0.0:9", "-address",
		"-t", "--t=sse", "--transport", "-transport=sse", "--endpoint-path=/x", "-endpoint-path"} {
		ctx, cancel := context.WithCancelCause(context.Background())
		_, err := mcpproxy.StartGrafana(ctx, cancel, "/nonexistent/mcp-grafana", "http://grafana.test", "test-key", "127.0.0.1:8000",
			[]string{"--usage-stats=disabled", a})
		cancel(nil)
		name, _, _ := strings.Cut(a, "=")
		if err == nil || !strings.Contains(err.Error(), name) || strings.Contains(err.Error(), "start mcp-grafana") {
			t.Errorf("%q: err = %v", a, err)
		}
		if v, ok := strings.CutPrefix(a, name+"="); ok && strings.Contains(err.Error(), v) {
			t.Errorf("%q: error echoes the value: %v", a, err)
		}
	}
}

func TestProxy_UpstreamAuthRejection502(t *testing.T) {
	for _, st := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://upstream.test/.well-known/oauth-protected-resource"`)
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(st)
			_, _ = io.WriteString(w, "upstream says no")
		}))
		var out bytes.Buffer
		srv, tok := stack(t, "posthog", up.URL+"/mcp", nil, &out)
		resp := do(t, http.MethodPost, srv.URL+"/mcp/posthog", tok, `{}`, nil)
		if resp.StatusCode != http.StatusBadGateway || resp.Header.Get("WWW-Authenticate") != "" {
			t.Fatalf("%d: got %d, WWW-Authenticate %q", st, resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
		}
		code, msg := errBody(t, resp)
		if code != "upstream_unavailable" || !strings.Contains(msg, "credentials") {
			t.Errorf("%d: body %q %q", st, code, msg)
		}
		srv.Close()
		up.Close()
		if lines := auditLines(t, &out); len(lines) != 1 || lines[0]["status"] != float64(502) {
			t.Errorf("%d: audit = %v", st, lines)
		}
	}
}

func TestProxy_WWWAuthenticateAlwaysDropped(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="upstream"`)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(up.Close)
	var out bytes.Buffer
	srv, tok := stack(t, "posthog", up.URL+"/mcp", nil, &out)
	resp := do(t, http.MethodPost, srv.URL+"/mcp/posthog", tok, `{}`, nil)
	if resp.StatusCode != 200 || resp.Header.Get("WWW-Authenticate") != "" {
		t.Fatalf("got %d, WWW-Authenticate %q", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
}
