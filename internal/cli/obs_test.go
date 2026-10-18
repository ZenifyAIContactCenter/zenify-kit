package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"gopkg.in/yaml.v3"
)

func routeOK(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(api.ObsRouteResponse{
		Service: "svc-a", Route: "GET /x", Env: "prd", SinceEffective: "24h0m0s",
		Stats:   &api.ObsStats{Count: 7, P95: 0.5},
		Samples: api.ObsSamples{Slowest: []api.ObsSample{{TraceID: "abc", DurationMS: 12}}},
	})
}

func TestObs_BadArgs(t *testing.T) {
	mp, hits := brokerFixture(t, map[string]http.HandlerFunc{"/obs/route": routeOK, "/obs/trace": routeOK})
	cases := map[string][]string{
		"neither":      {"svc-a", "--prd"},
		"both":         {"svc-a", "--route", "GET /x", "--trace", "abc", "--prd"},
		"no env":       {"svc-a", "--route", "GET /x"},
		"both envs":    {"svc-a", "--route", "GET /x", "--prd", "--stg"},
		"route no id":  {"--route", "GET /x", "--prd"},
		"trace no env": {"--trace", "abc"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := execCmd(newObsCmd(), append([]string{"--manifest", mp}, args...)...)
			if exitcode.Code(err) != exitcode.BadArgs {
				t.Errorf("code = %d (err %v), want 2", exitcode.Code(err), err)
			}
		})
	}
	if *hits != 0 {
		t.Errorf("broker hit %d times on bad args", *hits)
	}
}

func TestObs_RouteRequestAndYAML(t *testing.T) {
	var req api.ObsRouteRequest
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/obs/route": func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&req)
		routeOK(w, r)
	}})
	out, _, err := execCmd(newObsCmd(), "--manifest", mp, "svc-a", "--route", "GET /x", "--stg", "--since", "6h")
	if err != nil {
		t.Fatal(err)
	}
	if req != (api.ObsRouteRequest{Service: "svc-a", Route: "GET /x", Since: "6h", Env: "stg"}) {
		t.Errorf("request = %+v", req)
	}
	var got map[string]any
	if err := yaml.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout not YAML: %v\n%s", err, out)
	}
	if got["since_effective"] != "24h0m0s" || !strings.Contains(out, "trace_id: abc") {
		t.Errorf("yaml keys should follow json names:\n%s", out)
	}
}

func TestObs_TraceDefaultSinceAndJSON(t *testing.T) {
	var req api.ObsTraceRequest
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/obs/trace": func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(api.ObsTraceResponse{TraceID: "00ab", SpanCount: 3})
	}})
	out, _, err := execCmd(newObsCmd(), "--manifest", mp, "--trace", "ab", "--prd", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if req.TraceID != "ab" || req.Env != "prd" {
		t.Errorf("request = %+v", req)
	}
	var resp api.ObsTraceResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil || resp.SpanCount != 3 || resp.TraceID != "00ab" {
		t.Errorf("stdout not the JSON response (%v):\n%s", err, out)
	}
}

func TestObs_Truncated(t *testing.T) {
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/obs/trace": func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.ObsTraceResponse{Truncated: true, Omitted: map[string]int{"spans": 4, "logs": 2}})
	}})
	out, errOut, err := execCmd(newObsCmd(), "--manifest", mp, "--trace", "ab", "--prd")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "obs: response truncated (logs=2, spans=4)") {
		t.Errorf("stderr = %q", errOut)
	}
	if strings.Contains(out, "truncated (") {
		t.Errorf("warning leaked to stdout:\n%s", out)
	}
}

func TestObs_TruncatedNoOmitted(t *testing.T) {
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/obs/route": func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.ObsRouteResponse{Truncated: true})
	}})
	_, errOut, err := execCmd(newObsCmd(), "--manifest", mp, "svc-a", "--route", "GET /x", "--prd")
	if err != nil || !strings.Contains(errOut, "obs: response truncated") {
		t.Errorf("err=%v stderr=%q", err, errOut)
	}
}

func TestObs_Plain404IsNotAvailable(t *testing.T) {
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/obs/route": func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "404 page not found", http.StatusNotFound)
	}})
	_, _, err := execCmd(newObsCmd(), "--manifest", mp, "svc-a", "--route", "GET /x", "--prd")
	if exitcode.Code(err) != exitcode.NoCatalog || err == nil || !strings.Contains(err.Error(), "obs not available on this broker") {
		t.Errorf("err = %v (code %d)", err, exitcode.Code(err))
	}
}

func TestObs_NoBrokerInManifest(t *testing.T) {
	_, _, err := execCmd(newObsCmd(), "--manifest", noBrokerManifest(t), "--trace", "ab", "--prd")
	if exitcode.Code(err) != exitcode.NoCatalog || err == nil || !strings.Contains(err.Error(), "obs not available on this broker") {
		t.Errorf("err = %v (code %d)", err, exitcode.Code(err))
	}
}

func TestObs_TraceNotFoundIsFail(t *testing.T) {
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/obs/trace": func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(api.Error{Error: api.CodeTraceNotFound, Message: "trace gone from tempo"})
	}})
	_, _, err := execCmd(newObsCmd(), "--manifest", mp, "--trace", "ab", "--prd")
	if exitcode.Code(err) != exitcode.Fail || err == nil || !strings.Contains(err.Error(), "trace gone from tempo") {
		t.Errorf("err = %v (code %d)", err, exitcode.Code(err))
	}
}

func TestObs_Broker400IsBadArgs(t *testing.T) {
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/obs/route": func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(api.Error{Error: api.CodeUnknownService, Message: "unknown service nope"})
	}})
	_, _, err := execCmd(newObsCmd(), "--manifest", mp, "nope", "--route", "GET /x", "--prd")
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("code = %d (err %v)", exitcode.Code(err), err)
	}
}
