package catalog

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const fixture = `version: 1
datasources:
  prd: {prometheus: uid-prd-prom, loki: uid-prd-loki, tempo: uid-prd-tempo}
  stg: {prometheus: uid-stg-prom, loki: uid-stg-loki, tempo: uid-stg-tempo}
labels:
  loki_app: app
  loki_host: host
services:
  - id: svc-a
    repo: repo-a
    prd:
      loki: [svc-a-prd]
      tempo: [svc-a]
      prometheus_job: [job-a]
      metrics: [metric_a_total]
      host: host-a
    stg:
      loki: [svc-a-stg]
  - id: svc-b
    repo: repo-b
    prd: {loki: [svc-b-prd]}
datastores:
  - id: db-a
    repo: ""
    stg: {prometheus_job: [job-db]}
attributes:
  tenant: tenant.id
posthog:
  project_id: "42"
  covers: [svc-a]
ignore:
  prd:
    loki: [noise-prd]
    tempo: [noise-tempo]
    prometheus_job: [noise-job]
  stg:
    loki: [noise-stg]
`

func TestParse_RoundTrip(t *testing.T) {
	c, err := Parse([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	want := &Catalog{
		Version: 1,
		Datasources: map[string]Datasources{
			"prd": {Prometheus: "uid-prd-prom", Loki: "uid-prd-loki", Tempo: "uid-prd-tempo"},
			"stg": {Prometheus: "uid-stg-prom", Loki: "uid-stg-loki", Tempo: "uid-stg-tempo"},
		},
		Labels: Labels{LokiApp: "app", LokiHost: "host"},
		Services: []Entry{
			{ID: "svc-a", Repo: "repo-a",
				Prd: &EnvNames{Loki: []string{"svc-a-prd"}, Tempo: []string{"svc-a"}, PrometheusJob: []string{"job-a"}, Metrics: []string{"metric_a_total"}, Host: "host-a"},
				Stg: &EnvNames{Loki: []string{"svc-a-stg"}}},
			{ID: "svc-b", Repo: "repo-b", Prd: &EnvNames{Loki: []string{"svc-b-prd"}}},
		},
		Datastores: []Entry{{ID: "db-a", Stg: &EnvNames{PrometheusJob: []string{"job-db"}}}},
		Attributes: map[string]string{"tenant": "tenant.id"},
		PostHog:    PostHog{ProjectID: "42", Covers: []string{"svc-a"}},
		Ignore: map[string]Names{
			"prd": {Loki: []string{"noise-prd"}, Tempo: []string{"noise-tempo"}, PrometheusJob: []string{"noise-job"}},
			"stg": {Loki: []string{"noise-stg"}},
		},
	}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("got %+v\nwant %+v", c, want)
	}
}

func TestParse_RejectsVersion2(t *testing.T) {
	if _, err := Parse([]byte("version: 2\n")); err == nil {
		t.Fatal("want error for version 2")
	}
}

func TestFilter_ByIDAndEnv(t *testing.T) {
	c, err := Parse([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	f, err := c.Filter("svc-a", "stg")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Services) != 1 || f.Services[0].ID != "svc-a" || len(f.Datastores) != 0 {
		t.Fatalf("entries: %+v / %+v", f.Services, f.Datastores)
	}
	if f.Services[0].Prd != nil || f.Services[0].Stg == nil {
		t.Errorf("env filter: prd=%v stg=%v", f.Services[0].Prd, f.Services[0].Stg)
	}
	if _, ok := f.Datasources["prd"]; ok || len(f.Datasources) != 1 {
		t.Errorf("datasources: %+v", f.Datasources)
	}
	if len(c.Services) != 2 || c.Services[0].Prd == nil {
		t.Error("Filter mutated the receiver")
	}
}

func TestFilter_UnknownID(t *testing.T) {
	c, err := Parse([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Filter("nope", "")
	var u *UnknownIDError
	if !errors.As(err, &u) {
		t.Fatalf("want *UnknownIDError, got %v", err)
	}
	if !reflect.DeepEqual(u.Known, []string{"svc-a", "svc-b", "db-a"}) {
		t.Errorf("Known = %v", u.Known)
	}
	if !strings.Contains(err.Error(), "svc-b") {
		t.Errorf("message should list known ids: %v", err)
	}
}

func TestParse_ToleratesUnknownKeys(t *testing.T) {
	src := "version: 1\nfuture_key: x\nservices:\n  - id: svc-a\n    new_field: y\n    prd: {loki: [a], extra: 1}\n"
	c, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Services) != 1 || c.Services[0].ID != "svc-a" || c.Services[0].Prd.Loki[0] != "a" {
		t.Errorf("got %+v", c.Services)
	}
}

func TestFilter_UnknownEnv(t *testing.T) {
	c, _ := Parse([]byte(fixture))
	if _, err := c.Filter("", "dev"); err == nil {
		t.Fatal("want error for env dev")
	}
}

func TestFilter_NarrowsIgnoreToEnv(t *testing.T) {
	c, _ := Parse([]byte(fixture))
	f, err := c.Filter("", "stg")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Ignore) != 1 || !reflect.DeepEqual(f.Ignore["stg"].Loki, []string{"noise-stg"}) {
		t.Errorf("Ignore = %+v", f.Ignore)
	}
}
