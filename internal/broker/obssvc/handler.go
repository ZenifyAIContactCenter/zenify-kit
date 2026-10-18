package obssvc

import (
	"net/http"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/catalogsvc"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/grafanaproxy"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/perlogin"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/catalog"
)

const (
	maxBody     = 4 << 10
	maxResponse = 32 << 10
	// maxUpstream caps each Grafana response the broker reads.
	maxUpstream = 4 << 20
	// parallel is how many Grafana calls one request runs at once.
	parallel = 3
)

// Handler serves POST /obs/route and POST /obs/trace over the catalog and
// Grafana's datasource proxy. Both routes must sit behind auth.RequireAudited
// and share one per-login slot.
type Handler struct {
	cat        *catalogsvc.Handler
	grafanaURL string
	token      string
	g          *grafanaproxy.Client
	now        func() time.Time
	audit      *audit.Logger // nil = no audit rows
	slots      perlogin.Slots
}

// Option tunes a Handler.
type Option func(*Handler)

// WithAudit makes each request write one audit row.
func WithAudit(l *audit.Logger) Option { return func(h *Handler) { h.audit = l } }

// WithClock replaces time.Now (tests).
func WithClock(now func() time.Time) Option { return func(h *Handler) { h.now = now } }

// WithGrafanaBaseURL points the Grafana client elsewhere (tests).
func WithGrafanaBaseURL(u string) Option { return func(h *Handler) { h.grafanaURL = u } }

// New builds the obs handler over cat's catalog and the Grafana at grafanaURL.
func New(cat *catalogsvc.Handler, grafanaURL, grafanaToken string, opts ...Option) *Handler {
	h := &Handler{cat: cat, grafanaURL: grafanaURL, token: grafanaToken, now: time.Now}
	for _, o := range opts {
		o(h)
	}
	h.g = grafanaproxy.New(h.grafanaURL, h.token)
	return h
}

// Trace is the POST /obs/trace handler.
func (h *Handler) Trace() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		auth.WriteError(w, http.StatusNotImplemented, api.CodeBrokerMisconfigured, "obs trace is not implemented yet")
	})
}

// findEntry returns the service or datastore with id, or nil.
func findEntry(c *catalog.Catalog, id string) *catalog.Entry {
	for _, list := range [][]catalog.Entry{c.Services, c.Datastores} {
		for i := range list {
			if list[i].ID == id {
				return &list[i]
			}
		}
	}
	return nil
}

// envNames returns the entry's names on env, or nil.
func envNames(e *catalog.Entry, env string) *catalog.EnvNames {
	if env == catalog.Envs[1] {
		return e.Stg
	}
	return e.Prd
}

// quote escapes \ and " so s can sit inside a TraceQL or LogQL string literal.
func quote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}
