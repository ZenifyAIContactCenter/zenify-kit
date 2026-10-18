package catalogsvc

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/grafanaproxy"
)

const maxLabelBytes = 4 << 20

// grafana reads label values through Grafana's datasource proxy with a
// service-account token. Errors name the status or the cause only, never the
// token or the upstream body.
type grafana struct{ c *grafanaproxy.Client }

func newGrafana(baseURL, token string) *grafana {
	return &grafana{c: grafanaproxy.New(baseURL, token)}
}

// lokiLabelValues: start/end are nanosecond epochs.
func (g *grafana) lokiLabelValues(ctx context.Context, uid, label string, start, end time.Time) ([]string, error) {
	var out struct{ Data []string }
	err := g.get(ctx, uid, "/loki/api/v1/label/"+url.PathEscape(label)+"/values",
		strconv.FormatInt(start.UnixNano(), 10), strconv.FormatInt(end.UnixNano(), 10), &out)
	return out.Data, err
}

// promLabelValues: start/end are unix seconds.
func (g *grafana) promLabelValues(ctx context.Context, uid, label string, start, end time.Time) ([]string, error) {
	var out struct{ Data []string }
	err := g.get(ctx, uid, "/api/v1/label/"+url.PathEscape(label)+"/values",
		strconv.FormatInt(start.Unix(), 10), strconv.FormatInt(end.Unix(), 10), &out)
	return out.Data, err
}

// tempoTagValues: start/end are epoch seconds.
func (g *grafana) tempoTagValues(ctx context.Context, uid, tag string, start, end time.Time) ([]string, error) {
	var out struct {
		TagValues []struct{ Value string } `json:"tagValues"`
	}
	err := g.get(ctx, uid, "/api/v2/search/tag/"+url.PathEscape(tag)+"/values",
		strconv.FormatInt(start.Unix(), 10), strconv.FormatInt(end.Unix(), 10), &out)
	vals := make([]string, 0, len(out.TagValues))
	for _, v := range out.TagValues {
		vals = append(vals, v.Value)
	}
	return vals, err
}

func (g *grafana) get(ctx context.Context, uid, path, start, end string, out any) error {
	return g.c.Get(ctx, uid, path, url.Values{"start": {start}, "end": {end}}, maxLabelBytes, out)
}
