package apply

import (
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
)

// A v0.34 user jumping straight to v0.36 still has the five legacy names in
// settings; the move keeps them although the manifest dropped them.
func TestMigrationKeys_KeepsLegacyMySQL(t *testing.T) {
	got := MigrationKeys([]string{"MONGO_URL", "MYSQL_URL", "MYSQL_HOST"})
	want := "MONGO_URL,MYSQL_URL,MYSQL_HOST,MYSQL_PORT,MYSQL_USER,MYSQL_PASSWORD,MYSQL_DATABASE"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v, want %s", got, want)
	}

	f := newMigrateFixture(t)
	writeFileT(t, f.wsSettings, `{"env":{"MYSQL_PASSWORD":"fake-pw"}}`)
	res, err := MigrateSettingsEnv(f.paths(), MigrationKeys([]string{"MONGO_URL"}), f.envPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.Moved, ",") != "MYSQL_PASSWORD" {
		t.Fatalf("moved = %v, want MYSQL_PASSWORD", res.Moved)
	}
}

// legacyFive is a fixture with a password carrying @ : / and a user with @.
var legacyFive = map[string]string{
	"MYSQL_HOST":     "db.internal",
	"MYSQL_PORT":     "3307",
	"MYSQL_USER":     "ro@team",
	"MYSQL_PASSWORD": "p@ss:w/rd-fake",
	"MYSQL_DATABASE": "db_acd",
}

func writeEnvT(t *testing.T, path string, vals map[string]string, extra ...string) {
	t.Helper()
	f, err := devenv.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range append(append([]string{}, LegacyMySQLKeys...), extra...) {
		if v, ok := vals[k]; ok {
			f.Set(k, v)
			f.SetDesc(k, "old description of "+k)
		}
	}
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
}

// SC-15
func TestMergeMySQLURL_RoundTripAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zenify", ".env")
	writeEnvT(t, path, legacyFive)

	res, err := MergeMySQLURL(path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Merged || res.Conflict || len(res.Missing) != 0 {
		t.Fatalf("res = %+v, want Merged", res)
	}
	f, _ := devenv.Load(path)
	for _, k := range LegacyMySQLKeys {
		if _, ok := f.Get(k); ok {
			t.Errorf("%s still in file", k)
		}
	}
	raw, ok := f.Get("MYSQL_URL")
	if !ok {
		t.Fatal("MYSQL_URL not written")
	}
	body := readFileT(t, path)
	if strings.Contains(body, "old description of MYSQL_") {
		t.Errorf("legacy description lines left: %q", body)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal("written MYSQL_URL does not parse")
	}
	pw, _ := u.User.Password()
	got := map[string]string{
		"MYSQL_HOST": u.Hostname(), "MYSQL_PORT": u.Port(), "MYSQL_USER": u.User.Username(),
		"MYSQL_PASSWORD": pw, "MYSQL_DATABASE": strings.TrimPrefix(u.Path, "/"),
	}
	for k, v := range legacyFive {
		if got[k] != v {
			t.Errorf("%s round-trips to %q, want %q", k, got[k], v)
		}
	}
	if u.Scheme != "mysql" {
		t.Errorf("scheme = %q", u.Scheme)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}

	before := readFileT(t, path)
	res, err = MergeMySQLURL(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res, MySQLURLResult{}) {
		t.Errorf("second run = %+v, want no-op", res)
	}
	if readFileT(t, path) != before {
		t.Error("second run changed the file")
	}
}

func TestMergeMySQLURL_PortOptional(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	vals := map[string]string{}
	for k, v := range legacyFive {
		vals[k] = v
	}
	delete(vals, "MYSQL_PORT")
	writeEnvT(t, path, vals)
	if res, err := MergeMySQLURL(path); err != nil || !res.Merged {
		t.Fatalf("res = %+v err = %v", res, err)
	}
	f, _ := devenv.Load(path)
	raw, _ := f.Get("MYSQL_URL")
	u, _ := url.Parse(raw)
	if u.Hostname() != "db.internal" || u.Port() != "" {
		t.Errorf("host %q port %q, want db.internal and no port", u.Hostname(), u.Port())
	}
}

func TestMergeMySQLURL_ExistingEqualDropsLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	writeEnvT(t, path, legacyFive)
	f, _ := devenv.Load(path)
	f.Set("MYSQL_URL", buildMySQLURL(legacyFive))
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	res, err := MergeMySQLURL(path)
	if err != nil || !res.Dropped || res.Merged {
		t.Fatalf("res = %+v err = %v, want Dropped", res, err)
	}
	f, _ = devenv.Load(path)
	if strings.Join(f.Keys(), ",") != "MYSQL_URL" {
		t.Errorf("keys = %v, want only MYSQL_URL", f.Keys())
	}
}

func TestMergeMySQLURL_ConflictChangesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	writeEnvT(t, path, legacyFive)
	f, _ := devenv.Load(path)
	f.Set("MYSQL_URL", "mysql://other:fake@h/db")
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	before := readFileT(t, path)
	res, err := MergeMySQLURL(path)
	if err != nil || !res.Conflict {
		t.Fatalf("res = %+v err = %v, want Conflict", res, err)
	}
	if readFileT(t, path) != before {
		t.Error("conflict changed the file")
	}
}

func TestMergeMySQLURL_MissingChangesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	writeEnvT(t, path, map[string]string{"MYSQL_HOST": "h", "MYSQL_USER": "u", "MYSQL_PASSWORD": ""})
	before := readFileT(t, path)
	res, err := MergeMySQLURL(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.Missing, ",") != "MYSQL_PASSWORD,MYSQL_DATABASE" || res.Merged || res.Conflict {
		t.Fatalf("res = %+v, want Missing MYSQL_PASSWORD,MYSQL_DATABASE", res)
	}
	if readFileT(t, path) != before {
		t.Error("missing key changed the file")
	}
}

// A file that never held the legacy keys is left alone, with nothing to report.
func TestMergeMySQLURL_NoLegacyNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	writeFileT(t, path, "MONGO_URL='fake'\n")
	res, err := MergeMySQLURL(path)
	if err != nil || !reflect.DeepEqual(res, MySQLURLResult{}) {
		t.Fatalf("res = %+v err = %v, want zero", res, err)
	}
	if _, err := MergeMySQLURL(filepath.Join(t.TempDir(), "absent", ".env")); err != nil {
		t.Fatalf("missing file: %v", err)
	}
}
