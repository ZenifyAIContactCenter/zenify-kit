// Package dbread ports the workspace's `db_read` POSIX shell tool
// (~/.local/bin/db_read) to a portable Go implementation (FR-033).
//
// It shells out to `mongosh`/`mysql` so output matches the CLIs the bash tool
// used. No subcommand takes free-form code: Mongo reads are built from
// structured flags by BuildMongoScript, and SQL is allowlisted by CheckSQL
// and run inside WrapSQL's READ ONLY transaction.
//
// FR-041: credentials ($MONGO_URL, $MYSQL_PASSWORD) are read into memory and
// passed to child processes ONLY through the child env (ZNF_MONGO_URL,
// MYSQL_PWD), never on argv. No code path in this package prints, logs, or
// echoes a connection string or password. Error diagnostics use the hardcoded
// host 103.209.34.57 only, never the URL.
package dbread

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// Options configures one Run. Stdout/Stderr and the run/Env seams let tests
// inject without spawning real processes or touching a real settings file.
//
// Options is always passed to Run by pointer: Run redirects where the run
// seam writes (to a filter buffer or a line cap) and the injected run closure
// reads o.Stdout dynamically at call time — that only works if Run and the
// closure share the same Options.
type Options struct {
	Cmd, Arg     string
	Filter       string
	Projection   string
	Sort         string
	Pipeline     string
	Limit        int
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

// errCapReached is returned by capWriter once output arrives past the cap; the
// runner then stops the child, so the rest is never transferred.
var errCapReached = errors.New("output cap reached")

// truncNotice is the stderr line printed when a read is cut at --limit.
const truncNotice = "… đã cắt ở %d dòng (--limit, tối đa %d)\n" //znf:allow-lang

// capWriter forwards at most limit lines to w. The first byte past the
// limit-th line marks it truncated and fails the write with errCapReached
// (a short write), which stops the child's output copy.
type capWriter struct {
	w         io.Writer
	left      int
	truncated bool
}

func newCapWriter(w io.Writer, limit int) *capWriter { return &capWriter{w: w, left: limit} }

func (c *capWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		if c.left == 0 {
			c.truncated = true
			return n - len(p), errCapReached
		}
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			_, err := c.w.Write(p)
			return n, err
		}
		if _, err := c.w.Write(p[:i+1]); err != nil {
			return n, err
		}
		c.left--
		p = p[i+1:]
	}
	return n, nil
}

// Truncated reports whether any output arrived after the limit-th line.
func (c *capWriter) Truncated() bool { return c.truncated }

const mongoHost = "103.209.34.57"

// argRequired lists the subcommands that need a collection, table or query arg.
var argRequired = map[string]string{
	"doc":       "doc cần tên collection — chạy 'zenify db-read collections' trước", //znf:allow-lang
	"count":     "count cần tên collection",                                         //znf:allow-lang
	"find":      "find cần tên collection",                                          //znf:allow-lang
	"aggregate": "aggregate cần tên collection",                                     //znf:allow-lang
	"explain":   "explain cần tên collection",                                       //znf:allow-lang
	"sql":       "sql cần một câu SELECT/SHOW/DESCRIBE/EXPLAIN",                     //znf:allow-lang
}

// unusedFlags lists, per subcommand, the query flags it never reads; passing
// one is refused rather than silently ignored.
var unusedFlags = map[string][]string{
	"collections": {"--filter", "--projection", "--sort", "--pipeline"},
	"tables":      {"--filter", "--projection", "--sort", "--pipeline"},
	"doc":         {"--filter", "--projection", "--sort", "--pipeline"},
	"sql":         {"--filter", "--projection", "--sort", "--pipeline"},
	"count":       {"--projection", "--sort"},
}

func badArgs(msg string) error {
	return exitcode.New(exitcode.BadArgs, errors.New("zenify db-read: "+msg))
}

func printUsage(w io.Writer) {
	for _, l := range []string{
		"Usage:",
		"  db-read collections [substr]          list Mongo collections in 3csoft, optionally filtered",
		"  db-read tables [substr]               list MySQL tables in db_acd, optionally filtered",
		"  db-read doc <collection>              one document, to read field names",
		"  db-read count <collection> [--filter] count (estimated without --filter)",
		"  db-read find <collection>             --filter --projection --sort --limit (default 50, max 200)",
		"  db-read aggregate <collection>        --pipeline '[...]' ($out/$merge refused), --limit",
		"  db-read explain <collection>          executionStats, no --limit (runs the read query, max 30 s) for --filter/--sort or --pipeline",
		"  db-read sql '<query>'                 one SELECT/WITH/SHOW/DESCRIBE/EXPLAIN in a READ ONLY txn",
		"  output cut at --limit prints a notice on stderr; --pipeline never combines with --filter/--projection/--sort",
		"  distinct: db-read aggregate <coll> --pipeline '[{\"$group\":{\"_id\":\"$field\"}}]'",
	} {
		_, _ = fmt.Fprintln(w, l)
	}
}

// Run dispatches one db_read subcommand. It validates every argument and
// builds the query BEFORE resolving credentials, so a bad request never
// reaches a database. Output goes to o.Stdout via the injected run seam —
// Run itself never returns secret values in an error string.
func Run(o *Options) error {
	switch o.Cmd {
	case "collections", "tables", "doc", "count", "find", "aggregate", "explain", "sql":
	case "eval":
		return badArgs("db-read eval đã bị gỡ — dùng find/aggregate/count/explain") //znf:allow-lang
	default:
		printUsage(o.Stderr)
		return badArgs(fmt.Sprintf("unknown subcommand %q", o.Cmd))
	}
	if msg, ok := argRequired[o.Cmd]; ok && strings.TrimSpace(o.Arg) == "" {
		return badArgs(msg)
	}
	limit := o.Limit
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 0 || limit > MaxLimit {
		return badArgs(fmt.Sprintf("--limit phải trong khoảng 1..%d", MaxLimit)) //znf:allow-lang
	}
	if o.Cmd == "aggregate" && strings.TrimSpace(o.Pipeline) == "" {
		return badArgs("aggregate cần --pipeline '[...]'") //znf:allow-lang
	}
	flagVal := map[string]string{"--filter": o.Filter, "--projection": o.Projection, "--sort": o.Sort, "--pipeline": o.Pipeline}
	for _, f := range unusedFlags[o.Cmd] {
		if strings.TrimSpace(flagVal[f]) != "" {
			return badArgs(fmt.Sprintf("%s không dùng cho %s", f, o.Cmd)) //znf:allow-lang
		}
	}
	hasPipeline := strings.TrimSpace(o.Pipeline) != ""
	hasFind := strings.TrimSpace(o.Filter+o.Projection+o.Sort) != ""
	if hasPipeline && (hasFind || o.Cmd == "find" || o.Cmd == "count") {
		return badArgs("--pipeline không đi cùng --filter/--projection/--sort, và chỉ dùng cho aggregate/explain") //znf:allow-lang
	}

	var script, sqlStmt string
	switch o.Cmd {
	case "tables":
		sqlStmt = WrapSQLNoLimit("SHOW TABLES")
	case "sql":
		if err := CheckSQL(o.Arg); err != nil {
			return badArgs(err.Error())
		}
		sqlStmt = WrapSQL(o.Arg, limit)
	default:
		var err error
		script, err = BuildMongoScript(MongoQuery{
			Op: o.Cmd, Coll: o.Arg, Filter: o.Filter, Projection: o.Projection,
			Sort: o.Sort, Pipeline: o.Pipeline, Limit: limit,
		})
		if err != nil {
			return badArgs(err.Error())
		}
	}

	if o.Env == nil {
		o.Env = os.Getenv
	}
	creds, _ := LoadCreds(o.Env, o.SettingsPath)
	mongoURL := creds["MONGO_URL"]
	if mongoURL == "" {
		if o.SettingsPath == "" {
			_, _ = fmt.Fprintln(o.Stderr, "zenify db-read: chưa có workspace — chạy `zenify up` trước") //znf:allow-lang
			return fmt.Errorf("zenify db-read: no workspace")
		}
		_, _ = fmt.Fprintf(o.Stderr, "zenify db-read: $MONGO_URL is not set and %s did not supply it.\n", o.SettingsPath)
		_, _ = fmt.Fprintln(o.Stderr, "zenify db-read: it belongs in the env block of that file, next to E2E_PASSWORD.")
		return fmt.Errorf("zenify db-read: MONGO_URL not set")
	}

	if script != "" {
		err := o.run("mongosh", []string{"--nodb", "--quiet", "--eval", script}, []string{MongoURLEnv + "=" + mongoURL}, "")
		if err != nil {
			_, _ = fmt.Fprintln(o.Stderr, "zenify db-read: mongosh failed. Separate network from credential before theorising:")
			_, _ = fmt.Fprintf(o.Stderr, "zenify db-read:   nc -z %s 27017   # needs no credential\n", mongoHost)
			_, _ = fmt.Fprintln(o.Stderr, "zenify db-read: port open  -> auth, or a stale $MONGO_URL in settings.local.json")
			_, _ = fmt.Fprintln(o.Stderr, "zenify db-read: port shut  -> network. This host is reachable directly, no VPN.")
		}
		return err
	}

	host := creds["MYSQL_HOST"]
	if host == "" {
		_, _ = fmt.Fprintln(o.Stderr, "zenify db-read: $MYSQL_HOST is not set — same source as $MONGO_URL, see above.")
		return fmt.Errorf("zenify db-read: MYSQL_HOST not set")
	}
	port := creds["MYSQL_PORT"]
	if port == "" {
		port = "3306"
	}
	database := creds["MYSQL_DATABASE"]
	if database == "" {
		database = "db_acd"
	}
	mysqlRun := func() error {
		return o.run("mysql", []string{
			"--binary-mode", "--quick", "--connect-timeout=10",
			"-h", host, "-P", port, "-u", creds["MYSQL_USER"],
			"-N", "-B", "-e", sqlStmt, database,
		}, []string{"MYSQL_PWD=" + creds["MYSQL_PASSWORD"]}, "")
	}

	stdout := o.Stdout
	defer func() { o.Stdout = stdout }()
	if o.Cmd == "tables" {
		// Not capped: the substring filter must see every table.
		var buf bytes.Buffer
		o.Stdout = &buf
		if err := mysqlRun(); err != nil {
			return err
		}
		_, _ = fmt.Fprint(stdout, filterLines(buf.String(), o.Arg))
		return nil
	}
	cw := newCapWriter(stdout, limit)
	o.Stdout = cw
	// Once the cap is hit the runner kills mysql, so its error is expected:
	// truncation is a success.
	if err := mysqlRun(); err != nil && !cw.Truncated() {
		return err
	}
	if cw.Truncated() {
		_, _ = fmt.Fprintf(o.Stderr, truncNotice, limit, MaxLimit)
	}
	return nil
}
