package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

// Fake values only — never a real credential.
const (
	fakeV1 = "fake-mongo-v1"
	fakeV2 = "fake-mongo-v2"
)

func writeFileT(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFileT(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- test temp path
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// migrateFixture is a workspace settings file plus one repo settings file.
type migrateFixture struct {
	ws, wsSettings, repoSettings, envPath string
}

func newMigrateFixture(t *testing.T) migrateFixture {
	t.Helper()
	ws := t.TempDir()
	return migrateFixture{
		ws:           ws,
		wsSettings:   filepath.Join(ws, ".claude", "settings.local.json"),
		repoSettings: filepath.Join(ws, "svc", ".claude", "settings.local.json"),
		envPath:      filepath.Join(ws, ".zenify", ".env"),
	}
}

func (f migrateFixture) paths() []string { return []string{f.wsSettings, f.repoSettings} }

// SC-8
func TestMigrate_MovesAndDeletes(t *testing.T) {
	f := newMigrateFixture(t)
	writeFileT(t, f.wsSettings, `{"env":{"MONGO_URL":"`+fakeV1+`","OTHER":"x"},"model":"opus"}`)
	res, err := MigrateSettingsEnv(f.paths(), []string{"MONGO_URL", "E2E_EMAIL"}, f.envPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Moved, []string{"MONGO_URL"}) {
		t.Errorf("Moved = %v, want [MONGO_URL]", res.Moved)
	}
	if got := readFileT(t, f.envPath); !strings.Contains(got, "MONGO_URL='"+fakeV1+"'") {
		t.Errorf(".zenify/.env missing MONGO_URL line:\n%s", got)
	}
	env := readEnv(t, f.wsSettings)
	if _, ok := env["MONGO_URL"]; ok {
		t.Errorf("MONGO_URL still in settings env: %v", env)
	}
	if env["OTHER"] != "x" {
		t.Errorf("OTHER lost: %v", env)
	}
	if !strings.Contains(readFileT(t, f.wsSettings), `"model": "opus"`) {
		t.Errorf("unrelated top-level key lost:\n%s", readFileT(t, f.wsSettings))
	}
	fi, err := os.Stat(f.envPath)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf(".zenify/.env mode = %v err=%v, want 0600", fi, err)
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	f := newMigrateFixture(t)
	writeFileT(t, f.wsSettings, `{"env":{"MONGO_URL":"`+fakeV1+`","OTHER":"x"}}`)
	writeFileT(t, f.repoSettings, `{"env":{"MONGO_URL":""}}`)
	if _, err := MigrateSettingsEnv(f.paths(), []string{"MONGO_URL"}, f.envPath, nil); err != nil {
		t.Fatal(err)
	}
	before := map[string]string{}
	for _, p := range append(f.paths(), f.envPath) {
		before[p] = readFileT(t, p)
	}
	res, err := MigrateSettingsEnv(f.paths(), []string{"MONGO_URL"}, f.envPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved)+len(res.Removed)+len(res.Conflicts) != 0 {
		t.Errorf("second run result = %+v, want empty", res)
	}
	for p, b := range before {
		if readFileT(t, p) != b {
			t.Errorf("%s changed on second run", p)
		}
	}
}

// SC-12
func TestMigrate_RemovesEmptyPlaceholders(t *testing.T) {
	f := newMigrateFixture(t)
	writeFileT(t, f.repoSettings, `{"env":{"MONGO_URL":"","KEEP":"k"}}`)
	res, err := MigrateSettingsEnv(f.paths(), []string{"MONGO_URL"}, f.envPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Removed, []string{"MONGO_URL"}) {
		t.Errorf("Removed = %v, want [MONGO_URL]", res.Removed)
	}
	if len(res.Moved) != 0 {
		t.Errorf("Moved = %v, want empty", res.Moved)
	}
	env := readEnv(t, f.repoSettings)
	if _, ok := env["MONGO_URL"]; ok || env["KEEP"] != "k" {
		t.Errorf("env after = %v", env)
	}
	if _, err := os.Stat(f.envPath); !os.IsNotExist(err) {
		t.Errorf(".zenify/.env must not be created for a placeholder-only run (err=%v)", err)
	}
}

// FR-9.2
func TestMigrate_Conflict(t *testing.T) {
	f := newMigrateFixture(t)
	ws := `{"env":{"MONGO_URL":"` + fakeV1 + `"}}`
	repo := `{"env":{"MONGO_URL":"` + fakeV2 + `"}}`
	writeFileT(t, f.wsSettings, ws)
	writeFileT(t, f.repoSettings, repo)
	res, err := MigrateSettingsEnv(f.paths(), []string{"MONGO_URL"}, f.envPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Conflicts, []string{"MONGO_URL"}) {
		t.Errorf("Conflicts = %v, want [MONGO_URL]", res.Conflicts)
	}
	if readFileT(t, f.wsSettings) != ws || readFileT(t, f.repoSettings) != repo {
		t.Errorf("a conflicting key must leave every file unchanged")
	}
	if _, err := os.Stat(f.envPath); !os.IsNotExist(err) {
		t.Errorf(".zenify/.env must not be written for a conflict (err=%v)", err)
	}
}

// FR-9.2: .zenify/.env counts as one of the places; a different value there is a conflict.
func TestMigrate_EnvFileDiffersIsConflict(t *testing.T) {
	f := newMigrateFixture(t)
	writeFileT(t, f.envPath, "MONGO_URL='fake-a'\n")
	body := `{"env":{"MONGO_URL":"fake-b"}}`
	writeFileT(t, f.wsSettings, body)
	res, err := MigrateSettingsEnv(f.paths(), []string{"MONGO_URL"}, f.envPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Conflicts, []string{"MONGO_URL"}) || len(res.Moved) != 0 {
		t.Errorf("result = %+v, want Conflicts [MONGO_URL] only", res)
	}
	if got := readFileT(t, f.envPath); got != "MONGO_URL='fake-a'\n" {
		t.Errorf(".zenify/.env changed")
	}
	if readFileT(t, f.wsSettings) != body {
		t.Errorf("settings changed although the key conflicts with .zenify/.env")
	}
}

// Same value in .zenify/.env and settings: drop it from settings, .env untouched.
func TestMigrate_EnvFileSameValueRemoves(t *testing.T) {
	f := newMigrateFixture(t)
	writeFileT(t, f.envPath, "MONGO_URL='fake-a'\n")
	writeFileT(t, f.wsSettings, `{"env":{"MONGO_URL":"fake-a"}}`)
	res, err := MigrateSettingsEnv(f.paths(), []string{"MONGO_URL"}, f.envPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) != 0 || !reflect.DeepEqual(res.Moved, []string{"MONGO_URL"}) {
		t.Errorf("result = %+v, want Moved [MONGO_URL]", res)
	}
	if got := readFileT(t, f.envPath); got != "MONGO_URL='fake-a'\n" {
		t.Errorf(".zenify/.env changed")
	}
	if _, ok := readEnv(t, f.wsSettings)["MONGO_URL"]; ok {
		t.Errorf("settings key must be removed")
	}
}

// The settings rewrite keeps another key's value bytes (no HTML escaping of &).
func TestMigrate_RewritePreservesValueBytes(t *testing.T) {
	f := newMigrateFixture(t)
	other := "mongodb://h:27017/db?replicaSet=rs0&authSource=admin"
	writeFileT(t, f.wsSettings, `{"env":{"MONGO_URL":"`+fakeV1+`","OTHER_URL":"`+other+`"}}`)
	if _, err := MigrateSettingsEnv(f.paths(), []string{"MONGO_URL"}, f.envPath, nil); err != nil {
		t.Fatal(err)
	}
	raw := readFileT(t, f.wsSettings)
	if !strings.Contains(raw, "&authSource") || strings.Contains(raw, `\u0026`) {
		t.Errorf("& was escaped or lost:\n%s", raw)
	}
	if readEnv(t, f.wsSettings)["OTHER_URL"] != other {
		t.Errorf("OTHER_URL changed")
	}
}

// FR-9.6
func TestMigrate_OwnedFingerprint(t *testing.T) {
	f := newMigrateFixture(t)
	writeFileT(t, f.wsSettings, `{"env":{"MONGO_URL":"`+fakeV1+`"}}`)
	writeFileT(t, f.repoSettings, `{"env":{"MONGO_URL":""}}`)
	owned := &managed.Manifest{}
	if err := owned.Record(f.wsSettings); err != nil { // unchanged since the kit wrote it
		t.Fatal(err)
	}
	if err := owned.Record(f.repoSettings); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, f.repoSettings, `{"env":{"MONGO_URL":"","MINE":"m"}}`) // user edited after
	stale, _ := owned.Get(f.repoSettings)

	if _, err := MigrateSettingsEnv(f.paths(), []string{"MONGO_URL"}, f.envPath, owned); err != nil {
		t.Fatal(err)
	}
	e, _ := owned.Get(f.wsSettings)
	if e.SHA256 != managed.Fingerprint([]byte(readFileT(t, f.wsSettings))) {
		t.Errorf("unchanged owned file must be re-recorded with its new content")
	}
	if e2, _ := owned.Get(f.repoSettings); e2.SHA256 != stale.SHA256 {
		t.Errorf("user-modified owned file must keep its old fingerprint")
	}
}

func TestMigrate_NoValuesInResult(t *testing.T) {
	f := newMigrateFixture(t)
	writeFileT(t, f.wsSettings, `{"env":{"MONGO_URL":"`+fakeV1+`","E2E_EMAIL":"fake-e2e@x"}}`)
	writeFileT(t, f.repoSettings, `{"env":{"MONGO_URL":"`+fakeV2+`","E2E_EMAIL":"","POSTHOG_API_KEY":""}}`)
	res, err := MigrateSettingsEnv(f.paths(), []string{"MONGO_URL", "E2E_EMAIL", "POSTHOG_API_KEY"}, f.envPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	all := append(append(append([]string{}, res.Moved...), res.Removed...), res.Conflicts...)
	if len(all) == 0 {
		t.Fatal("expected a non-empty result")
	}
	for _, s := range all {
		for _, v := range []string{fakeV1, fakeV2, "fake-e2e@x"} {
			if strings.Contains(s, v) {
				t.Errorf("result entry %q carries a value", s)
			}
		}
	}
}

// FR-9.1 ordering: when .zenify/.env cannot be written, no settings file changes.
func TestMigrate_EnvWriteFailureLeavesSettingsUntouched(t *testing.T) {
	f := newMigrateFixture(t)
	body := `{"env":{"MONGO_URL":"` + fakeV1 + `"}}`
	writeFileT(t, f.wsSettings, body)
	writeFileT(t, filepath.Join(f.ws, ".zenify"), "not a dir") // .zenify is a file → Save fails
	if _, err := MigrateSettingsEnv(f.paths(), []string{"MONGO_URL"}, f.envPath, nil); err == nil {
		t.Fatal("want an error when .zenify/.env cannot be written")
	}
	if readFileT(t, f.wsSettings) != body {
		t.Errorf("settings modified although .zenify/.env write failed")
	}
}

// A malformed settings file fails closed: nothing is written anywhere.
func TestMigrate_MalformedSettingsWritesNothing(t *testing.T) {
	f := newMigrateFixture(t)
	body := `{"env":{"MONGO_URL":"` + fakeV1 + `"}}`
	writeFileT(t, f.wsSettings, body)
	writeFileT(t, f.repoSettings, `{not json`)
	if _, err := MigrateSettingsEnv(f.paths(), []string{"MONGO_URL"}, f.envPath, nil); err == nil {
		t.Fatal("want an error for a malformed settings file")
	}
	if readFileT(t, f.wsSettings) != body {
		t.Errorf("settings modified although another file was malformed")
	}
	if _, err := os.Stat(f.envPath); !os.IsNotExist(err) {
		t.Errorf(".zenify/.env written although a settings file was malformed")
	}
}

// FR-8.3
func TestLeftoverEnvKeys(t *testing.T) {
	f := newMigrateFixture(t)
	writeFileT(t, f.wsSettings, `{"env":{"MONGO_URL":"`+fakeV1+`","EMPTY":"","NUL":null,"PORT":3306,"ON":true}}`)
	got, err := LeftoverEnvKeys(f.paths())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{f.wsSettings: {"MONGO_URL", "ON", "PORT"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LeftoverEnvKeys = %v, want %v", got, want)
	}
}

// SC-13
func TestKitDeny_EnsureRemove(t *testing.T) {
	ws := t.TempDir()
	writeSettings(t, ws, `{"permissions":{"deny":["Read(/secret)"]}}`)
	readDeny := func() []string {
		var doc struct {
			Permissions struct {
				Deny []string `json:"deny"`
			} `json:"permissions"`
		}
		if err := json.Unmarshal([]byte(readFileT(t, settingsPath(ws))), &doc); err != nil {
			t.Fatal(err)
		}
		return doc.Permissions.Deny
	}
	n, err := EnsureKitDeny(ws, false)
	if err != nil || n != 1 {
		t.Fatalf("ensure: n=%d err=%v", n, err)
	}
	if got := readDeny(); !reflect.DeepEqual(got, []string{"Read(/secret)", "Read(/.zenify/.env)"}) {
		t.Errorf("deny after ensure = %v", got)
	}
	if _, err := os.Stat(filepath.Join(ws, ".zenify", "kit-deny.json")); err != nil {
		t.Errorf("deny record missing: %v", err)
	}
	if n, err := RemoveKitDeny(ws, false); err != nil || n != 1 {
		t.Fatalf("remove: n=%d err=%v", n, err)
	}
	if got := readDeny(); !reflect.DeepEqual(got, []string{"Read(/secret)"}) {
		t.Errorf("deny after remove = %v", got)
	}
}
