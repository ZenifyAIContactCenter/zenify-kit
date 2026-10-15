package apply

import (
	"fmt"
	"net"
	"net/url"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
)

// LegacyMySQLKeys are the five keys MYSQL_URL replaced in v0.36.0 (FR-12).
// They left the manifest, but the settings→.env move still carries them so a
// user skipping v0.35 lands them in .zenify/.env, where MergeMySQLURL joins them.
var LegacyMySQLKeys = []string{"MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DATABASE"}

// MigrationKeys is the manifest key list plus every legacy key it lacks.
func MigrationKeys(manifestKeys []string) []string {
	out := append([]string{}, manifestKeys...)
	seen := map[string]bool{}
	for _, k := range out {
		seen[k] = true
	}
	for _, k := range LegacyMySQLKeys {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}

// MySQLURLResult says what MergeMySQLURL did. Flags and key names only, never values.
type MySQLURLResult struct {
	Merged   bool     // MYSQL_URL written from the legacy keys, legacy keys deleted
	Dropped  bool     // MYSQL_URL already equal to the joined value; legacy keys deleted
	Conflict bool     // MYSQL_URL differs from the joined value; file untouched
	Missing  []string // required legacy keys absent or empty; file untouched
}

// buildMySQLURL joins the legacy values; user and password are percent-encoded.
func buildMySQLURL(v map[string]string) string {
	host := v["MYSQL_HOST"]
	if p := v["MYSQL_PORT"]; p != "" {
		host = net.JoinHostPort(host, p)
	}
	u := url.URL{Scheme: "mysql", User: url.UserPassword(v["MYSQL_USER"], v["MYSQL_PASSWORD"]), Host: host, Path: "/" + v["MYSQL_DATABASE"]}
	return u.String()
}

// MergeMySQLURL joins the legacy MYSQL_* keys in the credential file at
// envPath into one MYSQL_URL (FR-12.3). Nothing happens when no legacy key is
// present, so a second run is a no-op.
func MergeMySQLURL(envPath string) (MySQLURLResult, error) {
	f, err := devenv.Load(envPath)
	if err != nil {
		return MySQLURLResult{}, err
	}
	vals := map[string]string{}
	present := false
	for _, k := range LegacyMySQLKeys {
		if v, ok := f.Get(k); ok {
			present = true
			vals[k] = v
		}
	}
	if !present {
		return MySQLURLResult{}, nil
	}
	var res MySQLURLResult
	for _, k := range LegacyMySQLKeys {
		if k != "MYSQL_PORT" && vals[k] == "" {
			res.Missing = append(res.Missing, k)
		}
	}
	if len(res.Missing) > 0 {
		return res, nil
	}
	joined := buildMySQLURL(vals)
	cur, ok := f.Get("MYSQL_URL") // an empty value counts as absent
	switch {
	case !ok || cur == "":
		f.Set("MYSQL_URL", joined)
		res.Merged = true
	case cur == joined:
		res.Dropped = true
	default:
		return MySQLURLResult{Conflict: true}, nil
	}
	for _, k := range LegacyMySQLKeys {
		f.Delete(k)
	}
	if err := f.Save(envPath); err != nil {
		return MySQLURLResult{}, fmt.Errorf("write %s: %w", envPath, err)
	}
	return res, nil
}
