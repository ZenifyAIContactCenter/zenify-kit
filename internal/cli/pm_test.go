package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func lookAll(s string) (string, error) { return "/bin/" + s, nil }

func TestDetectPM(t *testing.T) {
	const pj = `{"name":"x"}`
	tests := []struct {
		name    string
		files   map[string]string
		look    func(string) (string, error)
		want    string
		reason  string
		wantErr string
	}{
		{"packageManager wins over lockfile", map[string]string{"package.json": `{"packageManager":"pnpm@9.1.0"}`, "yarn.lock": ""}, lookAll, "pnpm", "packageManager field in package.json", ""},
		{"pnpm lock", map[string]string{"package.json": pj, "pnpm-lock.yaml": ""}, lookAll, "pnpm", "pnpm-lock.yaml", ""},
		{"yarn lock", map[string]string{"package.json": pj, "yarn.lock": ""}, lookAll, "yarn", "yarn.lock", ""},
		{"npm lock", map[string]string{"package.json": pj, "package-lock.json": ""}, lookAll, "npm", "package-lock.json", ""},
		{"bun.lock", map[string]string{"package.json": pj, "bun.lock": ""}, lookAll, "bun", "bun lockfile", ""},
		{"bun.lockb", map[string]string{"package.json": pj, "bun.lockb": ""}, lookAll, "bun", "bun lockfile", ""},
		{"yarn beats npm", map[string]string{"package.json": pj, "yarn.lock": "", "package-lock.json": ""}, lookAll, "yarn", "yarn.lock", ""},
		{"invalid json falls back to lockfile", map[string]string{"package.json": `{oops`, "yarn.lock": ""}, lookAll, "yarn", "yarn.lock", ""},
		{"no signal", map[string]string{"package.json": pj}, lookAll, "", "", "no packageManager field and no lockfile"},
		{"not installed", map[string]string{"package.json": pj, "yarn.lock": ""}, func(string) (string, error) { return "", errors.New("nope") }, "", "", "is not installed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, tc.files)
			mgr, root, reason, err := detectPM(dir, tc.look)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want contains %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mgr != tc.want || reason != tc.reason || root != dir {
				t.Fatalf("got (%q,%q,%q), want (%q,%q,%q)", mgr, root, reason, tc.want, dir, tc.reason)
			}
		})
	}
}

func TestDetectPMNoPackageJSON(t *testing.T) {
	_, _, _, err := detectPM(t.TempDir(), lookAll)
	if err == nil || !strings.Contains(err.Error(), "no package.json") {
		t.Fatalf("err = %v", err)
	}
}

func TestDetectPMFromSubdir(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"package.json": `{}`, "yarn.lock": ""})
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	_, got, _, err := detectPM(sub, lookAll)
	if err != nil || got != root {
		t.Fatalf("root = %q err=%v, want %q", got, err, root)
	}
}

func TestPmCmdNoArgs(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"package.json": `{}`, "yarn.lock": ""})
	var out, errb bytes.Buffer
	code := runPM(nil, root, &out, &errb, lookAll)
	if code != 0 || out.String() != "yarn\n" {
		t.Fatalf("code=%d stdout=%q", code, out.String())
	}
	if !strings.Contains(errb.String(), "root: ") || !strings.Contains(errb.String(), "reason: ") {
		t.Fatalf("stderr=%q", errb.String())
	}
}

func TestPmCmdErrorExit1(t *testing.T) {
	var out, errb bytes.Buffer
	code := runPM(nil, t.TempDir(), &out, &errb, lookAll)
	if code != 1 || !strings.Contains(errb.String(), "pm: no package.json") {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
}
