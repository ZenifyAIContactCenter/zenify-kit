package catalogsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	grafanaTimeout = 30 * time.Second
	maxLabelBytes  = 4 << 20
)

// grafana reads label values through Grafana's datasource proxy with a
// service-account token. Errors name the status or the cause only, never the
// token or the upstream body.
type grafana struct {
	baseURL string
	token   string
	client  *http.Client
}

func newGrafana(baseURL, token string) *grafana {
	return &grafana{baseURL: strings.TrimRight(baseURL, "/"), token: token, client: &http.Client{Timeout: grafanaTimeout}}
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
	u := g.baseURL + "/api/datasources/proxy/uid/" + url.PathEscape(uid) + path +
		"?" + url.Values{"start": {start}, "end": {end}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return errors.New("grafana request could not be built")
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	resp, err := g.client.Do(req)
	if err != nil {
		return errors.New("grafana unreachable")
	}
	defer func() {
		// Drain a bounded tail so the keep-alive connection can be reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("grafana returned %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxLabelBytes)).Decode(out); err != nil {
		return errors.New("grafana response could not be decoded")
	}
	return nil
}
