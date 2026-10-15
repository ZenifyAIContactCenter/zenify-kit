package mcpproxy

import (
	"net/url"
	"testing"
)

// The proxy itself must flush every write: FlushInterval -1 (FR-4.3).
func TestNewReverseProxy_FlushesImmediately(t *testing.T) {
	up, _ := url.Parse("http://127.0.0.1:1/mcp")
	if rp := newReverseProxy("grafana", up, nil); rp.FlushInterval != -1 {
		t.Fatalf("FlushInterval = %v, want -1", rp.FlushInterval)
	}
}
