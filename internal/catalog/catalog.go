// Package catalog defines the service catalog schema: which Grafana
// datasources, log/trace/metric names and PostHog project each service owns.
package catalog

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Envs are the environments a catalog describes.
var Envs = []string{"prd", "stg"}

// Catalog is the parsed catalog file. Its json tags are part of the broker
// wire contract (api.CatalogResponse embeds it): fields may only be added,
// never renamed or removed, as in internal/broker/api.
type Catalog struct {
	Version     int                    `yaml:"version" json:"version"`
	Datasources map[string]Datasources `yaml:"datasources" json:"datasources"`
	Labels      Labels                 `yaml:"labels" json:"labels"`
	Services    []Entry                `yaml:"services" json:"services"`
	Datastores  []Entry                `yaml:"datastores" json:"datastores"`
	Attributes  map[string]string      `yaml:"attributes" json:"attributes"`
	PostHog     PostHog                `yaml:"posthog" json:"posthog"`
	Ignore      map[string]Names       `yaml:"ignore" json:"ignore"`
}

// Datasources are the Grafana datasource uids of one environment.
type Datasources struct {
	Prometheus string `yaml:"prometheus" json:"prometheus"`
	Loki       string `yaml:"loki" json:"loki"`
	Tempo      string `yaml:"tempo" json:"tempo"`
}

// Labels names the Loki labels that carry the app and host.
type Labels struct {
	LokiApp  string `yaml:"loki_app" json:"loki_app"`
	LokiHost string `yaml:"loki_host" json:"loki_host"`
}

// Entry is one service or datastore; Prd/Stg are nil when absent there.
type Entry struct {
	ID   string    `yaml:"id" json:"id"`
	Repo string    `yaml:"repo" json:"repo"`
	Prd  *EnvNames `yaml:"prd,omitempty" json:"prd,omitempty"`
	Stg  *EnvNames `yaml:"stg,omitempty" json:"stg,omitempty"`
}

// EnvNames are the names an entry goes by in one environment.
type EnvNames struct {
	Loki          []string `yaml:"loki,omitempty" json:"loki,omitempty"`
	Tempo         []string `yaml:"tempo,omitempty" json:"tempo,omitempty"`
	PrometheusJob []string `yaml:"prometheus_job,omitempty" json:"prometheus_job,omitempty"`
	Metrics       []string `yaml:"metrics,omitempty" json:"metrics,omitempty"`
	Host          string   `yaml:"host,omitempty" json:"host,omitempty"`
}

// Names are names deliberately left out of the catalog (drift ignore list).
type Names struct {
	Loki          []string `yaml:"loki,omitempty" json:"loki,omitempty"`
	Tempo         []string `yaml:"tempo,omitempty" json:"tempo,omitempty"`
	PrometheusJob []string `yaml:"prometheus_job,omitempty" json:"prometheus_job,omitempty"`
}

// PostHog describes the PostHog project and which entries it covers.
type PostHog struct {
	ProjectID int64    `yaml:"project_id" json:"project_id"`
	Covers    []string `yaml:"covers" json:"covers"`
}

// Parse decodes a catalog. Unknown keys are tolerated (the file is versioned
// independently of this binary); any version but 1 is rejected.
func Parse(b []byte) (*Catalog, error) {
	var c Catalog
	dec := yaml.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	if c.Version != 1 {
		return nil, fmt.Errorf("catalog version %d is not supported (want 1)", c.Version)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

var (
	uidRE   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	labelRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// validate rejects datasource uids and label names that would change a
// Grafana proxy path (the broker builds one from each). Empty values stay
// legal: they mean "not set", which drift reports per source.
func (c *Catalog) validate() error {
	envs := make([]string, 0, len(c.Datasources))
	for env := range c.Datasources {
		envs = append(envs, env)
	}
	sort.Strings(envs)
	for _, env := range envs {
		d := c.Datasources[env]
		for _, f := range []struct{ key, v string }{{"prometheus", d.Prometheus}, {"loki", d.Loki}, {"tempo", d.Tempo}} {
			if f.v != "" && !uidRE.MatchString(f.v) {
				return fmt.Errorf("parse catalog: datasources.%s.%s is not a valid uid (want %s)", env, f.key, uidRE)
			}
		}
	}
	for _, f := range []struct{ key, v string }{{"loki_app", c.Labels.LokiApp}, {"loki_host", c.Labels.LokiHost}} {
		if f.v != "" && !labelRE.MatchString(f.v) {
			return fmt.Errorf("parse catalog: labels.%s is not a valid label name (want %s)", f.key, labelRE)
		}
	}
	return nil
}

// UnknownIDError reports an id that is in neither services nor datastores.
type UnknownIDError struct {
	ID    string
	Known []string
}

func (e *UnknownIDError) Error() string {
	return fmt.Sprintf("unknown id %q; known: %s", e.ID, strings.Join(e.Known, ", "))
}

// Filter returns a copy narrowed to one id (empty = all) and one env (empty =
// both). The receiver is not modified.
func (c *Catalog) Filter(id, env string) (*Catalog, error) {
	if env != "" && env != Envs[0] && env != Envs[1] {
		return nil, fmt.Errorf("unknown env %q; known: %s", env, strings.Join(Envs, ", "))
	}
	out := *c
	out.Services, out.Datastores = narrow(c.Services, id, env), narrow(c.Datastores, id, env)
	if id != "" && len(out.Services)+len(out.Datastores) == 0 {
		var known []string
		for _, e := range append(append([]Entry(nil), c.Services...), c.Datastores...) {
			known = append(known, e.ID)
		}
		return nil, &UnknownIDError{ID: id, Known: known}
	}
	if env != "" {
		out.Datasources = map[string]Datasources{}
		if d, ok := c.Datasources[env]; ok {
			out.Datasources[env] = d
		}
		out.Ignore = map[string]Names{}
		if n, ok := c.Ignore[env]; ok {
			out.Ignore[env] = n
		}
	}
	return &out, nil
}

func narrow(in []Entry, id, env string) []Entry {
	var out []Entry
	for _, e := range in {
		if id != "" && e.ID != id {
			continue
		}
		switch env {
		case "prd":
			e.Stg = nil
		case "stg":
			e.Prd = nil
		}
		out = append(out, e)
	}
	return out
}
