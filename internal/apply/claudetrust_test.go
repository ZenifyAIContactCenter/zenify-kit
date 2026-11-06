package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const trustWS = "/w/zenify"

func decodeAny(t *testing.T, p string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestEnsureClaudeTrust_FlipsAndPreserves(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".claude.json")
	orig := `{"numStartups": 42, "oauthAccount": {"email": "a<b>&c@x"}, "tips": [1, 2],
 "projects": {"/other": {"hasTrustDialogAccepted": false, "allowedTools": ["x"]},
  "` + trustWS + `": {"hasTrustDialogAccepted": false, "mcpServers": {"zenify-grafana": {"headersHelper": "zenify broker headers <a&b>"}}, "lastCost": 1.5}}}`
	if err := os.WriteFile(p, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := EnsureClaudeTrust(p, trustWS, false)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	var want map[string]any
	if err := json.Unmarshal([]byte(orig), &want); err != nil {
		t.Fatal(err)
	}
	want["projects"].(map[string]any)[trustWS].(map[string]any)["hasTrustDialogAccepted"] = true
	if got := decodeAny(t, p); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", fi.Mode().Perm())
	}
	// No HTML escaping of foreign values.
	if b, _ := os.ReadFile(p); strings.Contains(string(b), `\u003c`) || !strings.Contains(string(b), `<a&b>`) {
		t.Error("foreign value was HTML-escaped")
	}
	// Second call is a no-op: no write at all.
	before, _ := os.ReadFile(p)
	fi0, _ := os.Stat(p)
	if changed, err := EnsureClaudeTrust(p, trustWS, false); err != nil || changed {
		t.Fatalf("second call changed=%v err=%v", changed, err)
	}
	after, _ := os.ReadFile(p)
	fi1, _ := os.Stat(p)
	if string(before) != string(after) || !fi0.ModTime().Equal(fi1.ModTime()) {
		t.Error("second call rewrote the file")
	}
}

func TestEnsureClaudeTrust_CreatesEntryAndProjects(t *testing.T) {
	dir := t.TempDir()
	for name, orig := range map[string]string{
		"no-projects": `{"numStartups": 1}`,
		"no-entry":    `{"projects": {"/other": {"a": 1}}}`,
	} {
		p := filepath.Join(dir, name+".json")
		if err := os.WriteFile(p, []byte(orig), 0o600); err != nil {
			t.Fatal(err)
		}
		if changed, err := EnsureClaudeTrust(p, trustWS, false); err != nil || !changed {
			t.Fatalf("%s: changed=%v err=%v", name, changed, err)
		}
		got := decodeAny(t, p)
		if got["projects"].(map[string]any)[trustWS].(map[string]any)["hasTrustDialogAccepted"] != true {
			t.Errorf("%s: flag not set: %v", name, got)
		}
		if name == "no-projects" && got["numStartups"] != float64(1) {
			t.Errorf("%s: numStartups lost", name)
		}
		if name == "no-entry" && got["projects"].(map[string]any)["/other"].(map[string]any)["a"] != float64(1) {
			t.Errorf("%s: other project lost", name)
		}
	}
}

func TestEnsureClaudeTrust_DryRunWritesNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".claude.json")
	orig := `{"projects": {}}`
	if err := os.WriteFile(p, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, err := EnsureClaudeTrust(p, trustWS, true); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if b, _ := os.ReadFile(p); string(b) != orig {
		t.Errorf("dry run wrote %q", b)
	}
}

func TestEnsureClaudeTrust_MissingAndMalformed(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, ".claude.json")
	if _, err := EnsureClaudeTrust(missing, trustWS, false); err == nil {
		t.Fatal("missing file: want error")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("missing file was created: %v", err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"secret": "s3cr3t",`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := EnsureClaudeTrust(bad, trustWS, false)
	if err == nil {
		t.Fatal("malformed file: want error")
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("error leaks file content: %v", err)
	}
	if b, _ := os.ReadFile(bad); string(b) != `{"secret": "s3cr3t",` {
		t.Error("malformed file was rewritten")
	}
}

// A symlinked ~/.claude.json stays a symlink (the target is written), and a
// trailing slash on the workspace does not create a second project key.
func TestEnsureClaudeTrust_SymlinkAndCleanKey(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.json")
	link := filepath.Join(dir, ".claude.json")
	if err := os.WriteFile(target, []byte(`{"projects":{"`+trustWS+`":{"x":1}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if changed, err := EnsureClaudeTrust(link, trustWS+"/", false); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced: %v %v", fi, err)
	}
	projects := decodeAny(t, target)["projects"].(map[string]any)
	if len(projects) != 1 {
		t.Fatalf("projects = %v", projects)
	}
	if e := projects[trustWS].(map[string]any); e[claudeTrustKey] != true || e["x"] != float64(1) {
		t.Fatalf("entry = %v", e)
	}
}
