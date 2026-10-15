package mcpproxy_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

var jwtKey = []byte("jwt-test-key")

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
	tok, _ := auth.Issue(jwtKey, "octocat", time.Now())
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
	c, err := mcpproxy.StartGrafana(ctx, cancel, bin, "http://grafana.test", "test-key", "127.0.0.1:8001")
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

	wantArgs := []string{bin, "-t", "streamable-http", "--address", "127.0.0.1:8001", "--disable-write"}
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
	want := "arg:-t\narg:streamable-http\narg:--address\narg:127.0.0.1:8001\narg:--disable-write\nurl:http://grafana.test\ntok:test-key\n"
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
