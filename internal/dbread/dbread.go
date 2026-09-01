// Package dbread ports the workspace's `db_read` POSIX shell tool
// (~/.local/bin/db_read) to a portable Go implementation (FR-033).
//
// It shells out to `mongosh`/`mysql` rather than using a native Go DB
// driver: `eval` runs arbitrary mongosh JavaScript, which a native driver
// cannot evaluate, and shelling to the same CLIs the bash tool used
// guarantees byte-identical output for every other subcommand too.
//
// FR-041: credentials ($MONGO_URL, $MYSQL_PASSWORD) are read into memory
// and passed to child processes ONLY (mongosh URI arg; MYSQL_PWD in the
// child env). No code path in this package prints, logs, or echoes a
// connection string or password. Error diagnostics use the hardcoded host
// 103.209.34.57 only, never the URL.
package dbread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Options configures one Run. Stdout/Stderr and the run/Env seams let tests
// inject without spawning real processes or touching a real settings file.
//
// Options is always passed to Run by pointer: for the collections/tables
// substring filter, Run needs to redirect where the run seam writes (to an
// in-memory buffer, before filtering the result into the real Stdout), and
// the injected run closure reads o.Stdout dynamically at call time — that
// only works if Run and the closure share the same Options.
type Options struct {
	Cmd, Arg     string
	Stdout       io.Writer
	Stderr       io.Writer
	Env          func(string) string
	SettingsPath string
	run          func(name string, args []string, extraEnv []string, stdin string) error
}

// SetRun injects the process-spawning seam. Exported so the cobra command
// (internal/cli) can wire the real os/exec runner without this package
// exposing the run field directly; unit tests in this package set it too.
func (o *Options) SetRun(run func(name string, args []string, extraEnv []string, stdin string) error) {
	o.run = run
}

// LoadCreds resolves DB credentials: live env first, else the "env" block of
// the workspace settings.local.json (same store as bash db_read). Returns a
// map of the six keys that were found (MONGO_URL, MYSQL_HOST/PORT/USER/
// PASSWORD/DATABASE). Values are NEVER logged. A missing/unreadable
// settings file is not an error here — the caller decides whether the
// resulting absence of MONGO_URL is fatal.
func LoadCreds(getenv func(string) string, settingsPath string) (map[string]string, error) {
	keys := []string{"MONGO_URL", "MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DATABASE"}
	out := map[string]string{}
	for _, k := range keys {
		if v := getenv(k); v != "" {
			out[k] = v
		}
	}
	if out["MONGO_URL"] != "" {
		return out, nil
	}
	b, err := os.ReadFile(settingsPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		return out, nil // absent settings is not fatal here
	}
	var doc struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return out, nil // malformed → treat as absent, same as bash python failing open
	}
	for _, k := range keys {
		if out[k] == "" {
			if v := doc.Env[k]; v != "" {
				out[k] = v
			}
		}
	}
	return out, nil
}

// RefuseWrites reports true if stmt looks like a mutation and must be
// rejected. Advisory guard (a determined expression can evade a denylist)
// — the real rule is "writes go in a reviewed script". Ports bash
// refuse_writes exactly: match case-insensitively against the same verb
// list.
func RefuseWrites(stmt string) bool {
	s := strings.ToLower(stmt)
	needles := []string{
		"insert", "update", "delete", "remove", "replace", "drop", "create",
		"rename", "save(", "bulkwrite", "findandmodify", "collmod", "truncate",
		"alter", "grant", "revoke", "set ",
	}
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// filterLines keeps only the lines of out containing substr, case-insensitively.
// An empty substr returns out unchanged.
func filterLines(out, substr string) string {
	if substr == "" {
		return out
	}
	var keep []string
	low := strings.ToLower(substr)
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(strings.ToLower(l), low) {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

const mongoHost = "103.209.34.57"

// Run dispatches one db_read subcommand. It resolves creds, requires
// MONGO_URL, then dispatches on o.Cmd. Output goes to o.Stdout via the
// injected run seam — Run itself never returns secret values in an error
// string.
func Run(o *Options) error {
	if o.Env == nil {
		o.Env = os.Getenv
	}
	if o.SettingsPath == "" {
		o.SettingsPath = defaultSettingsPath(o.Env)
	}
	creds, _ := LoadCreds(o.Env, o.SettingsPath)
	mongoURL := creds["MONGO_URL"]
	if mongoURL == "" {
		_, _ = fmt.Fprintf(o.Stderr, "db_read: $MONGO_URL is not set and %s did not supply it.\n", o.SettingsPath)
		_, _ = fmt.Fprintln(o.Stderr, "db_read: it belongs in the env block of that file, next to E2E_PASSWORD.")
		return fmt.Errorf("db_read: MONGO_URL not set")
	}

	mongoEval := func(js string) error {
		err := o.run("mongosh", []string{mongoURL, "--quiet", "--eval", js}, nil, "")
		if err != nil {
			_, _ = fmt.Fprintln(o.Stderr, "db_read: mongosh failed. Separate network from credential before theorising:")
			_, _ = fmt.Fprintf(o.Stderr, "db_read:   nc -z %s 27017   # needs no credential\n", mongoHost)
			_, _ = fmt.Fprintln(o.Stderr, "db_read: port open  -> auth, or a stale $MONGO_URL in settings.local.json")
			_, _ = fmt.Fprintln(o.Stderr, "db_read: port shut  -> network. This host is reachable directly, no VPN.")
		}
		return err
	}

	mysqlRun := func(sql string) error {
		host := creds["MYSQL_HOST"]
		if host == "" {
			_, _ = fmt.Fprintln(o.Stderr, "db_read: $MYSQL_HOST is not set — same source as $MONGO_URL, see above.")
			return fmt.Errorf("db_read: MYSQL_HOST not set")
		}
		port := creds["MYSQL_PORT"]
		if port == "" {
			port = "3306"
		}
		database := creds["MYSQL_DATABASE"]
		if database == "" {
			database = "db_acd"
		}
		extraEnv := []string{"MYSQL_PWD=" + creds["MYSQL_PASSWORD"]}
		return o.run("mysql", []string{
			"-h", host, "-P", port, "-u", creds["MYSQL_USER"],
			"-N", "-B", "-e", sql, database,
		}, extraEnv, "")
	}

	// runFiltered redirects o.Stdout to a buffer for the duration of fn,
	// then writes the substring-filtered result to the real o.Stdout.
	runFiltered := func(fn func() error) error {
		real := o.Stdout
		var buf bytes.Buffer
		o.Stdout = &buf
		err := fn()
		o.Stdout = real
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(o.Stdout, filterLines(buf.String(), o.Arg))
		return nil
	}

	switch o.Cmd {
	case "collections":
		return runFiltered(func() error {
			return mongoEval(`db.getSiblingDB("3csoft").getCollectionInfos({type:"collection"}).map(c=>c.name).sort().forEach(n=>print(n))`)
		})
	case "tables":
		return runFiltered(func() error {
			return mysqlRun("SHOW TABLES")
		})
	case "doc":
		if o.Arg == "" {
			_, _ = fmt.Fprintln(o.Stderr, "db_read: doc needs a collection name — run 'db_read collections' first")
			return fmt.Errorf("db_read: arg required")
		}
		return mongoEval(fmt.Sprintf("printjson(db.getSiblingDB('3csoft').getCollection('%s').findOne())", o.Arg))
	case "count":
		if o.Arg == "" {
			_, _ = fmt.Fprintln(o.Stderr, "db_read: count needs a collection name")
			return fmt.Errorf("db_read: arg required")
		}
		return mongoEval(fmt.Sprintf("print(db.getSiblingDB('3csoft').getCollection('%s').estimatedDocumentCount())", o.Arg))
	case "eval":
		if o.Arg == "" {
			_, _ = fmt.Fprintln(o.Stderr, "db_read: eval needs a JS expression")
			return fmt.Errorf("db_read: arg required")
		}
		if RefuseWrites(o.Arg) {
			_, _ = fmt.Fprintln(o.Stderr, "db_read: refusing — this looks like a write.")
			_, _ = fmt.Fprintln(o.Stderr, "db_read: writes go in a script that connects and runs, never in a direct query.")
			return fmt.Errorf("db_read: refused write")
		}
		return mongoEval(fmt.Sprintf("db = db.getSiblingDB('3csoft'); %s", o.Arg))
	case "sql":
		if o.Arg == "" {
			_, _ = fmt.Fprintln(o.Stderr, "db_read: sql needs a query")
			return fmt.Errorf("db_read: arg required")
		}
		if RefuseWrites(o.Arg) {
			_, _ = fmt.Fprintln(o.Stderr, "db_read: refusing — this looks like a write.")
			_, _ = fmt.Fprintln(o.Stderr, "db_read: writes go in a script that connects and runs, never in a direct query.")
			return fmt.Errorf("db_read: refused write")
		}
		return mysqlRun(o.Arg)
	default:
		_, _ = fmt.Fprintln(o.Stderr, "Usage:")
		_, _ = fmt.Fprintln(o.Stderr, "  db-read collections            list every Mongo collection in 3csoft")
		_, _ = fmt.Fprintln(o.Stderr, "  db-read collections <substr>   only names containing <substr>")
		_, _ = fmt.Fprintln(o.Stderr, "  db-read tables                 list every MySQL table in db_acd")
		_, _ = fmt.Fprintln(o.Stderr, "  db-read tables <substr>        only names containing <substr>")
		_, _ = fmt.Fprintln(o.Stderr, "  db-read doc <collection>       findOne() from that collection")
		_, _ = fmt.Fprintln(o.Stderr, "  db-read count <collection>     estimated document count")
		_, _ = fmt.Fprintln(o.Stderr, "  db-read eval '<js>'            read-only mongosh expression, with `db` already = 3csoft")
		_, _ = fmt.Fprintln(o.Stderr, "  db-read sql '<query>'          read-only MySQL query")
		return fmt.Errorf("db_read: unknown subcommand %q", o.Cmd)
	}
}

func defaultSettingsPath(getenv func(string) string) string {
	home := getenv("HOME")
	return home + "/WorkingSpace/zenify/.claude/settings.local.json"
}
