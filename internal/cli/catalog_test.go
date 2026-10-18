package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/catalog"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func fakeCatalog() *catalog.Catalog {
	return &catalog.Catalog{
		Version: 1,
		Datasources: map[string]catalog.Datasources{
			"prd": {Prometheus: "prom-prd-uid"},
			"stg": {Prometheus: "prom-stg-uid"},
		},
		Services: []catalog.Entry{
			{ID: "svc-a", Repo: "repo-a", Prd: &catalog.EnvNames{Loki: []string{"svc-a-prd"}}, Stg: &catalog.EnvNames{Loki: []string{"svc-a-stg"}}},
			{ID: "svc-b", Repo: "repo-b", Prd: &catalog.EnvNames{Loki: []string{"svc-b-prd"}}},
		},
	}
}

func catalogHandler(c *catalog.Catalog, stale int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.CatalogResponse{Catalog: c, StaleAgeSeconds: stale})
	}
}

func TestCatalog_NotConfigured(t *testing.T) {
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/catalog": func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "404 page not found", http.StatusNotFound)
	}})
	_, _, err := execCmd(newCatalogCmd(), "--manifest", mp)
	if exitcode.Code(err) != exitcode.NoCatalog || err == nil || !strings.Contains(err.Error(), "no catalog configured") {
		t.Errorf("err = %v (code %d), want NoCatalog", err, exitcode.Code(err))
	}
}

func TestCatalog_NoBrokerInManifest(t *testing.T) {
	_, _, err := execCmd(newCatalogCmd(), "--manifest", noBrokerManifest(t))
	if exitcode.Code(err) != exitcode.NoCatalog || err == nil || !strings.Contains(err.Error(), "no catalog configured") {
		t.Errorf("err = %v (code %d), want NoCatalog", err, exitcode.Code(err))
	}
}

func TestCatalog_FilterIDEnv(t *testing.T) {
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/catalog": catalogHandler(fakeCatalog(), 0)})
	out, _, err := execCmd(newCatalogCmd(), "--manifest", mp, "--stg", "svc-a")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"svc-a", "svc-a-stg", "prom-stg-uid"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	for _, bad := range []string{"svc-b", "svc-a-prd", "prom-prd-uid"} {
		if strings.Contains(out, bad) {
			t.Errorf("stdout has %q:\n%s", bad, out)
		}
	}
}

func TestCatalog_UnknownID(t *testing.T) {
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/catalog": catalogHandler(fakeCatalog(), 0)})
	_, _, err := execCmd(newCatalogCmd(), "--manifest", mp, "nope")
	if exitcode.Code(err) != exitcode.BadArgs || err == nil || !strings.Contains(err.Error(), "svc-a") || !strings.Contains(err.Error(), "svc-b") {
		t.Errorf("err = %v (code %d), want BadArgs listing ids", err, exitcode.Code(err))
	}
}

func TestCatalog_PrdStgExclusive(t *testing.T) {
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/catalog": catalogHandler(fakeCatalog(), 0)})
	_, _, err := execCmd(newCatalogCmd(), "--manifest", mp, "--prd", "--stg")
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("err = %v (code %d), want BadArgs", err, exitcode.Code(err))
	}
}

func TestCatalog_StaleWarning(t *testing.T) {
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/catalog": catalogHandler(fakeCatalog(), 420)})
	out, errOut, err := execCmd(newCatalogCmd(), "--manifest", mp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "catalog is 420s old (broker could not refresh it)") {
		t.Errorf("stderr = %q", errOut)
	}
	if strings.Contains(out, "420") {
		t.Errorf("warning leaked to stdout: %s", out)
	}
}

func TestCatalogDrift_ExitCodes(t *testing.T) {
	var gotBody string
	cases := []struct {
		name string
		resp api.DriftResponse
		want int
	}{
		{"clean", api.DriftResponse{Envs: map[string]map[string]api.DriftResult{"prd": {"loki": {}}}}, exitcode.OK},
		{"unlisted", api.DriftResponse{Envs: map[string]map[string]api.DriftResult{"prd": {"loki": {Unlisted: []string{"new-app"}}}}}, exitcode.Drift},
		{"missing", api.DriftResponse{Envs: map[string]map[string]api.DriftResult{"stg": {"tempo": {Missing: []string{"gone-app"}}}}}, exitcode.Drift},
		{"error", api.DriftResponse{Envs: map[string]map[string]api.DriftResult{"prd": {"loki": {}, "tempo": {Error: "tempo down"}}}}, exitcode.Drift},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/catalog/drift": func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
				_ = json.NewEncoder(w).Encode(tc.resp)
			}})
			out, _, err := execCmd(newCatalogCmd(), "--manifest", mp, "drift")
			if exitcode.Code(err) != tc.want {
				t.Errorf("code = %d (err %v), want %d\n%s", exitcode.Code(err), err, tc.want, out)
			}
			if !strings.HasPrefix(out, "ENV") || !strings.Contains(out, "UNLISTED") {
				t.Errorf("no table header:\n%s", out)
			}
			if gotBody == "" || !strings.HasPrefix(gotBody, "{") {
				t.Errorf("request body = %q, want a JSON object", gotBody)
			}
		})
	}
}

func TestCatalogDrift_EnvFlagSentAndRows(t *testing.T) {
	var req api.DriftRequest
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/catalog/drift": func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(api.DriftResponse{Envs: map[string]map[string]api.DriftResult{"stg": {"loki": {Unlisted: []string{"x1", "x2"}}}}})
	}})
	out, _, err := execCmd(newCatalogCmd(), "--manifest", mp, "drift", "--stg")
	if exitcode.Code(err) != exitcode.Drift {
		t.Errorf("code = %d", exitcode.Code(err))
	}
	if req.Env != "stg" {
		t.Errorf("env sent = %q", req.Env)
	}
	if !strings.Contains(out, "stg") || !strings.Contains(out, "loki") || !strings.Contains(out, "x1,x2") {
		t.Errorf("table:\n%s", out)
	}
}
