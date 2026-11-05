package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDecideReviewerMatrix(t *testing.T) {
	for _, thread := range []string{"", "1"} {
		for _, zr := range []string{"agent", "codex"} {
			for _, onPath := range []bool{true, false} {
				env := map[string]string{"CODEX_THREAD_ID": thread, "ZNF_REVIEWER": zr}
				got, reason := decideReviewer(func(k string) string { return env[k] }, onPath)
				want, wantReason := "agent", ""
				if (thread != "" || zr == "codex") && onPath {
					want = "codex"
				} else if !onPath && (thread != "" || zr == "codex") {
					wantReason = "codex không có trên PATH" //znf:allow-lang
				} else {
					wantReason = "ZNF_REVIEWER=agent"
				}
				if got != want || reason != wantReason {
					t.Errorf("thread=%q zr=%q onPath=%v: got (%q,%q) want (%q,%q)", thread, zr, onPath, got, reason, want, wantReason)
				}
			}
		}
	}
}

func TestReviewerCmdPrints(t *testing.T) {
	var out bytes.Buffer
	printReviewer(&out, "agent", "ZNF_REVIEWER=agent")
	if out.String() != "agent (ZNF_REVIEWER=agent)\n" {
		t.Fatalf("got %q", out.String())
	}
	out.Reset()
	printReviewer(&out, "codex", "")
	if out.String() != "codex\n" {
		t.Fatalf("got %q", out.String())
	}
}

// SC-40: `reviewer set codex` writes env.ZNF_REVIEWER and keeps every other key;
// `reviewer set foo` exits non-zero and leaves the file byte-identical.
// reviewerWorkspace makes cwd a fresh workspace and returns its
// settings.local.json, where `reviewer set|get` keep ZNF_REVIEWER (FR-8.2).
func reviewerWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(ws)
	return filepath.Join(ws, ".claude", "settings.local.json")
}

func TestReviewerSetWritesSettingsEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := reviewerWorkspace(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	before := []byte(`{"model":"opus","env":{"ZNF_REVIEWER":"agent","OTHER":"1"}}`)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) error {
		root := NewRootCmd()
		var out, errb bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errb)
		root.SetArgs(args)
		return root.Execute()
	}

	if err := run("reviewer", "set", "foo"); err == nil {
		t.Fatal("reviewer set foo: want an error")
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, before) {
		t.Fatalf("set foo changed the file: %s", got)
	}

	if err := run("reviewer", "set", "codex"); err != nil {
		t.Fatalf("reviewer set codex: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Model string            `json:"model"`
		Env   map[string]string `json:"env"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Env["ZNF_REVIEWER"] != "codex" || got.Env["OTHER"] != "1" || got.Model != "opus" {
		t.Fatalf("settings after set codex: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("reviewer set wrote ~/.claude/settings.json: %v", err)
	}
}

// FR-17.3: `reviewer get` prints the stored env.ZNF_REVIEWER (what `reviewer set` writes),
// not the engine the running session picked; absent → agent; any other value → error.
func TestReviewerGetPrintsStoredValue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZNF_REVIEWER", "agent")
	t.Setenv("PATH", t.TempDir()) // no codex on PATH: `reviewer` alone would say agent
	path := reviewerWorkspace(t)
	get := func() (string, error) {
		root := NewRootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&bytes.Buffer{})
		root.SetArgs([]string{"reviewer", "get"})
		err := root.Execute()
		return out.String(), err
	}
	if out, err := get(); err != nil || out != "agent\n" {
		t.Fatalf("no settings file: %q %v, want agent", out, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"env":{"ZNF_REVIEWER":"codex"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := get(); err != nil || out != "codex\n" {
		t.Fatalf("stored codex: %q %v, want codex", out, err)
	}
	if err := os.WriteFile(path, []byte(`{"env":{"ZNF_REVIEWER":"foo"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := get(); err == nil {
		t.Fatal("stored foo: want an error")
	}
}
