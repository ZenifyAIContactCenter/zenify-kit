package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

type fakeRun struct {
	calls []string
	out   map[string]string
	errs  map[string]error
}

func (f *fakeRun) run(n string, args ...string) ([]byte, error) {
	c := n + " " + strings.Join(args, " ")
	f.calls = append(f.calls, c)
	return []byte(f.out[c]), f.errs[c]
}

func TestInstallPluginLocal_FreshInstall(t *testing.T) {
	zh, ws := t.TempDir(), t.TempDir()
	f := &fakeRun{out: map[string]string{"claude plugin list --json": "[]"}}
	if err := installPluginLocal(ws, zh, f.run); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"claude plugin list --json",
		"claude plugin marketplace add " + filepath.Join(zh, "plugin-marketplace") + " --scope local",
		"claude plugin install znf@zenify-kit --scope local",
	}
	if strings.Join(f.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v, want %v", f.calls, want)
	}
}

func enabledList(ws string) map[string]string {
	return map[string]string{"claude plugin list --json": fmt.Sprintf(
		`[{"id":"znf@zenify-kit","scope":"local","projectPath":%q,"enabled":true}]`, ws)}
}

func writeStamp(t *testing.T, ws, v string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "plugin-version"), []byte(v), 0o644); err != nil {
		t.Fatal(err)
	}
}

const updateCalls = "claude plugin marketplace update zenify-kit|claude plugin update znf@zenify-kit -s local"

func TestInstallPluginLocal_AlreadyEnabled(t *testing.T) {
	zh, ws := t.TempDir(), t.TempDir()
	writeStamp(t, ws, version.Current())
	f := &fakeRun{out: enabledList(ws)}
	if err := installPluginLocal(ws, zh, f.run); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("calls = %v, want only list", f.calls)
	}
}

// Enabled with no stamp: update first, then stamp.
func TestInstallPluginLocal_AlreadyEnabledNoStampUpdates(t *testing.T) {
	zh, ws := t.TempDir(), t.TempDir()
	f := &fakeRun{out: enabledList(ws)}
	if err := installPluginLocal(ws, zh, f.run); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.calls[1:], "|"); got != updateCalls {
		t.Fatalf("calls = %v", f.calls)
	}
	if b, _ := os.ReadFile(filepath.Join(ws, ".zenify", "plugin-version")); string(b) != version.Current() {
		t.Fatalf("stamp = %q", b)
	}
}

// Enabled with a stale stamp and a failing update: error, stamp stays stale.
func TestInstallPluginLocal_AlreadyEnabledStaleUpdateFails(t *testing.T) {
	zh, ws := t.TempDir(), t.TempDir()
	writeStamp(t, ws, "v0.0.1")
	f := &fakeRun{out: enabledList(ws), errs: map[string]error{"claude plugin update znf@zenify-kit -s local": fmt.Errorf("boom")}}
	if err := installPluginLocal(ws, zh, f.run); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want the update failure", err)
	}
	if b, _ := os.ReadFile(filepath.Join(ws, ".zenify", "plugin-version")); string(b) != "v0.0.1" {
		t.Fatalf("stamp = %q, want stale", b)
	}
}

func TestInstallPluginLocal_InstallFails(t *testing.T) {
	zh, ws := t.TempDir(), t.TempDir()
	f := &fakeRun{
		out:  map[string]string{"claude plugin list --json": "[]"},
		errs: map[string]error{"claude plugin install znf@zenify-kit --scope local": fmt.Errorf("boom")},
	}
	if err := installPluginLocal(ws, zh, f.run); err == nil {
		t.Fatal("want error")
	}
}

func TestInstallPluginLocal_MarketplaceAlreadyAddedIsOK(t *testing.T) {
	zh, ws := t.TempDir(), t.TempDir()
	add := "claude plugin marketplace add " + filepath.Join(zh, "plugin-marketplace") + " --scope local"
	f := &fakeRun{
		out:  map[string]string{"claude plugin list --json": "[]", add: "Marketplace 'zenify-kit' already exists"},
		errs: map[string]error{add: fmt.Errorf("exit status 1")},
	}
	if err := installPluginLocal(ws, zh, f.run); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 {
		t.Fatalf("calls = %v", f.calls)
	}
}

func TestInstallPluginLocal_SymlinkedProjectPathIsSame(t *testing.T) {
	zh := t.TempDir()
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "ln")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{real, link}, {link, real}} {
		writeStamp(t, real, version.Current())
		f := &fakeRun{out: enabledList(pair[0])}
		if err := installPluginLocal(pair[1], zh, f.run); err != nil {
			t.Fatal(err)
		}
		if len(f.calls) != 1 {
			t.Fatalf("list=%s arg=%s: calls = %v, want only list", pair[0], pair[1], f.calls)
		}
	}
}

func TestInstallPluginLocal_NotMatchingInstalls(t *testing.T) {
	ws := t.TempDir()
	other := t.TempDir()
	row := func(id, scope, path string, en bool) string {
		return fmt.Sprintf(`[{"id":%q,"scope":%q,"projectPath":%q,"enabled":%t}]`, id, scope, path, en)
	}
	cases := map[string]struct {
		out string
		err error
	}{
		"disabled":     {out: row("znf@zenify-kit", "local", ws, false)},
		"user scope":   {out: row("znf@zenify-kit", "user", ws, true)},
		"other path":   {out: row("znf@zenify-kit", "local", other, true)},
		"other id":     {out: row("x@y", "local", ws, true)},
		"not json":     {out: "garbage"},
		"list errored": {err: fmt.Errorf("boom")},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			zh := t.TempDir()
			f := &fakeRun{
				out:  map[string]string{"claude plugin list --json": c.out},
				errs: map[string]error{"claude plugin list --json": c.err},
			}
			if err := installPluginLocal(ws, zh, f.run); err != nil {
				t.Fatal(err)
			}
			if len(f.calls) != 3 || !strings.HasPrefix(f.calls[2], "claude plugin install") {
				t.Fatalf("calls = %v, want install", f.calls)
			}
		})
	}
}

func TestInstallPluginLocal_WritesStamp(t *testing.T) {
	zh, ws := t.TempDir(), t.TempDir()
	f := &fakeRun{out: map[string]string{"claude plugin list --json": "[]"}}
	if err := installPluginLocal(ws, zh, f.run); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(ws, ".zenify", "plugin-version"))
	if string(b) != version.Current() {
		t.Fatalf("stamp = %q", b)
	}
}

func TestResyncPluginLocal_StaleRunsUpdateAndStamps(t *testing.T) {
	zh, ws := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "plugin-version"), []byte("v0.0.1"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeRun{}
	var e bytes.Buffer
	resyncPluginLocal(ws, zh, f.run, &e)
	want := "claude plugin marketplace update zenify-kit|claude plugin update znf@zenify-kit -s local"
	if strings.Join(f.calls, "|") != want {
		t.Fatalf("calls = %v", f.calls)
	}
	b, _ := os.ReadFile(filepath.Join(ws, ".zenify", "plugin-version"))
	if string(b) != version.Current() {
		t.Fatalf("stamp = %q", b)
	}
	f.calls = nil
	resyncPluginLocal(ws, zh, f.run, &e)
	if len(f.calls) != 0 {
		t.Fatalf("current stamp ran %v", f.calls)
	}
}

func TestResyncPluginLocal_FailureKeepsStaleAndRetries(t *testing.T) {
	zh, ws := t.TempDir(), t.TempDir()
	writeStamp(t, ws, "v0.0.1")
	f := &fakeRun{errs: map[string]error{"claude plugin update znf@zenify-kit -s local": fmt.Errorf("boom")}}
	var e bytes.Buffer
	resyncPluginLocal(ws, zh, f.run, &e)
	if b, _ := os.ReadFile(filepath.Join(ws, ".zenify", "plugin-version")); string(b) != "v0.0.1" {
		t.Fatalf("stamp = %q despite failure", b)
	}
	if !strings.Contains(e.String(), "boom") {
		t.Fatalf("stderr = %q", e.String())
	}
	f.calls = nil
	resyncPluginLocal(ws, zh, f.run, &e)
	if len(f.calls) != 2 {
		t.Fatalf("no retry: %v", f.calls)
	}
}

// No stamp: the plugin was never installed locally here, so resync does nothing.
func TestResyncPluginLocal_NoStampIsNoop(t *testing.T) {
	zh, ws := t.TempDir(), t.TempDir()
	f := &fakeRun{}
	var e bytes.Buffer
	if err := resyncPluginLocal(ws, zh, f.run, &e); err != nil || len(f.calls) != 0 || e.Len() != 0 {
		t.Fatalf("err=%v calls=%v stderr=%q", err, f.calls, e.String())
	}
}
