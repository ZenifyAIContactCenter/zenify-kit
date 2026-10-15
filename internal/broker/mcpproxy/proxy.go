// Package mcpproxy forwards MCP Streamable HTTP traffic from /mcp/<name> to an
// upstream MCP server, swapping the client's broker JWT for the broker's own
// upstream credential and auditing every tools/call.
package mcpproxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
)

// statusClientClosed is audited when the client disconnects before the
// upstream answered (nginx's 499).
const statusClientClosed = 499

// stripClientHeaders drops what must not reach an upstream: the client's
// broker JWT and cookies, Cloudflare and proxy headers, and X-Grafana-*.
func stripClientHeaders(h http.Header) {
	for k := range h {
		ck := http.CanonicalHeaderKey(k)
		switch {
		case ck == "Authorization", ck == "Cookie", ck == "Forwarded", ck == "X-Real-Ip",
			strings.HasPrefix(ck, "Cf-"), strings.HasPrefix(ck, "X-Forwarded-"), strings.HasPrefix(ck, "X-Grafana-"):
			delete(h, k)
		}
	}
}

// maxBody caps the request body the proxy reads to find tools/call messages.
const maxBody = 4 << 20

// New proxies every request to upstream as-is: upstream's scheme, host and
// path, the client's query string. The client's credentials and edge/proxy
// headers are dropped (stripClientHeaders) before setHeaders (nil = none) adds
// the upstream credential. a (nil = stdout) gets one line per request plus one
// per tools/call message in the body.
func New(name string, upstream *url.URL, setHeaders func(h http.Header), a *audit.Logger) http.Handler {
	if a == nil {
		a = &audit.Logger{W: os.Stdout}
	}
	endpoint := "/mcp/" + name
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = upstream.Scheme
			pr.Out.URL.Host = upstream.Host
			pr.Out.URL.Path = upstream.Path
			pr.Out.URL.RawPath = upstream.RawPath
			pr.Out.URL.RawQuery = pr.In.URL.RawQuery
			pr.Out.Host = upstream.Host
			stripClientHeaders(pr.Out.Header)
			if setHeaders != nil {
				setHeaders(pr.Out.Header)
			}
		},
		FlushInterval: -1, // SSE must reach the client event by event (FR-4.3)
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) {
			if r.Context().Err() != nil { // the client went away: nobody reads a body
				if cw, ok := w.(*countingWriter); ok && cw.status == 0 {
					cw.status = statusClientClosed
				}
				return
			}
			auth.WriteError(w, http.StatusBadGateway, api.CodeUpstreamUnavailable, "MCP upstream "+name+" is unavailable")
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		login := auth.Login(r.Context())
		cw := &countingWriter{ResponseWriter: w}
		var calls []toolCall
		defer func() {
			ms, status := time.Since(start).Milliseconds(), cw.statusCode()
			for _, c := range calls {
				a.Log(audit.Entry{TS: start, Login: login, Endpoint: endpoint, Op: c.Name,
					Args: c.Args, MS: ms, Status: status})
			}
			a.Log(audit.Entry{TS: start, Login: login, Endpoint: endpoint, Op: r.Method,
				Bytes: cw.n, MS: ms, Status: status})
		}()

		if r.Body != nil && r.Body != http.NoBody {
			body, err := io.ReadAll(http.MaxBytesReader(cw, r.Body, maxBody))
			if err != nil {
				var tooBig *http.MaxBytesError
				if errors.As(err, &tooBig) {
					auth.WriteError(cw, http.StatusRequestEntityTooLarge, api.CodeBodyTooLarge, "MCP request body over 4 MiB")
				} else {
					auth.WriteError(cw, http.StatusBadRequest, api.CodeInvalidBody, "could not read the request body")
				}
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
			calls = toolCalls(body)
		}
		rp.ServeHTTP(cw, r)
	})
}

// PostHogHeaders sets PostHog's MCP credential, project and read-only mode
// (FR-4.2).
func PostHogHeaders(key, projectID string) func(http.Header) {
	return func(h http.Header) {
		h.Set("Authorization", "Bearer "+key)
		h.Set("x-posthog-project-id", projectID)
		h.Set("x-posthog-read-only", "true")
	}
}

type toolCall struct {
	Name string
	Args json.RawMessage
}

// toolCalls returns the tools/call messages of a JSON-RPC body, single or
// batch. Batch elements are decoded one by one, so a junk element hides only
// itself. Keys are matched exactly ("method", not "Method"), as a JS upstream
// reads them. An unparseable body has none; the upstream answers it.
func toolCalls(body []byte) []toolCall {
	msgs := []json.RawMessage{body}
	if t := bytes.TrimLeft(body, " \t\r\n"); len(t) > 0 && t[0] == '[' {
		if json.Unmarshal(t, &msgs) != nil {
			return nil
		}
	}
	var out []toolCall
	for _, raw := range msgs {
		if c, ok := asToolCall(raw); ok {
			out = append(out, c)
		}
	}
	return out
}

func asToolCall(raw json.RawMessage) (toolCall, bool) {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return toolCall{}, false
	}
	var method string
	if json.Unmarshal(m["method"], &method) != nil || method != "tools/call" {
		return toolCall{}, false
	}
	var c toolCall
	var params map[string]json.RawMessage
	if json.Unmarshal(m["params"], &params) == nil {
		_ = json.Unmarshal(params["name"], &c.Name)
		c.Args = params["arguments"]
	}
	return c, true
}

// countingWriter records the status and response bytes for the audit line.
// Unwrap lets http.ResponseController reach the real writer's Flush, so
// streaming still works through it.
type countingWriter struct {
	http.ResponseWriter
	status int
	n      int
}

func (c *countingWriter) WriteHeader(code int) {
	if c.status == 0 && code >= 200 { // 1xx is informational, not the final status
		c.status = code
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *countingWriter) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	n, err := c.ResponseWriter.Write(p)
	c.n += n
	return n, err
}

func (c *countingWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (c *countingWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

func (c *countingWriter) statusCode() int {
	if c.status == 0 {
		return http.StatusOK
	}
	return c.status
}
