package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakePlaneCreate(t *testing.T, gotBody *map[string]any) {
	t.Helper()
	const proj = "/api/v1/workspaces/zenify/projects/"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/users/me/":
			_, _ = io.WriteString(w, `{"id":"u-dev"}`)
		case r.Method == http.MethodGet && r.URL.Path == proj:
			_, _ = io.WriteString(w, `{"results":[{"id":"p-kb","identifier":"KB"},{"id":"p-zen","identifier":"ZEN"}],"next_page_results":false}`)
		case r.Method == http.MethodPost && r.URL.Path == proj+"p-zen/work-items/":
			if r.Header.Get("X-API-Key") != "pat-1" {
				http.Error(w, "no", http.StatusUnauthorized)
				return
			}
			_ = json.NewDecoder(r.Body).Decode(gotBody)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"wi-1","sequence_id":308,"name":"x"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	saved, savedClient := trustedBrokerManifest, trackPlaneClient
	yaml := "org: o\nrepos:\n  - name: r\n    url: u\nplane:\n  url: " + srv.URL + "\n  workspace: zenify\n"
	trustedBrokerManifest = func() ([]byte, string) { return []byte(yaml), "test-plane" }
	trackPlaneClient = srv.Client()
	t.Cleanup(func() { trustedBrokerManifest, trackPlaneClient = saved, savedClient })
}

func runPlane(ws string, args ...string) (string, error) {
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"plane"}, append(args, "--workspace", ws)...))
	err := root.Execute()
	return out.String(), err
}

func TestPlaneCreate(t *testing.T) {
	var body map[string]any
	fakePlaneCreate(t, &body)
	ws := planeTrackWS(t, "pat-1")
	out, err := runPlane(ws, "create", "Fix <the> thing", "--description", "a & b")
	if err != nil {
		t.Fatalf("create: %v (%s)", err, out)
	}
	want := "plane: ZEN-308 https://"
	if !strings.HasPrefix(out, want) || !strings.HasSuffix(strings.TrimSpace(out), "/zenify/browse/ZEN-308/") {
		t.Errorf("stdout = %q, want %q... /zenify/browse/ZEN-308/", out, want)
	}
	if body["name"] != "Fix <the> thing" {
		t.Errorf("name = %v", body["name"])
	}
	a, _ := body["assignees"].([]any)
	if len(a) != 1 || a[0] != "u-dev" {
		t.Errorf("assignees = %v, want [u-dev]", body["assignees"])
	}
	if body["description_html"] != "<p>a &amp; b</p>" {
		t.Errorf("description_html = %v", body["description_html"])
	}
}

func TestPlaneCreateUnknownProject(t *testing.T) {
	var body map[string]any
	fakePlaneCreate(t, &body)
	ws := planeTrackWS(t, "pat-1")
	if _, err := runPlane(ws, "create", "x", "--project", "NOPE"); err == nil || !strings.Contains(err.Error(), "NOPE") {
		t.Errorf("err = %v, want one naming the project", err)
	}
	if body != nil {
		t.Errorf("POST sent for an unknown project: %v", body)
	}
}

func TestPlaneCreateNoPAT(t *testing.T) {
	var body map[string]any
	fakePlaneCreate(t, &body)
	ws := planeTrackWS(t, "")
	_, err := runPlane(ws, "create", "x")
	if err == nil || !strings.Contains(err.Error(), "zenify up --apply") {
		t.Fatalf("err = %v, want one pointing at `zenify up --apply`", err)
	}
	if body != nil {
		t.Errorf("POST sent without a PAT")
	}
}
