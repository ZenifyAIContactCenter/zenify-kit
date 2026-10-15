// Package dbread ports the workspace's `db_read` POSIX shell tool
// (~/.local/bin/db_read) to a portable Go implementation (FR-033).
//
// It shells out to `mongosh`/`mysql` so output matches the CLIs the bash tool
// used. No subcommand takes free-form code: Mongo reads are built from
// structured flags by BuildMongoScript, and SQL is allowlisted by CheckSQL
// and run inside WrapSQL's READ ONLY transaction.
//
// FR-041: credentials ($MONGO_URL, the password inside $MYSQL_URL) are read
// into memory and passed to child processes ONLY through the child env
// (ZNF_MONGO_URL, MYSQL_PWD), never on argv. No code path in this package prints, logs, or
// echoes a connection string or password. Error diagnostics use the hardcoded
// host 103.209.34.57 only, never the URL.
package dbread

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// Options configures one Run. Stdout/Stderr and the run/Env seams let tests
// inject without spawning real processes or touching a real credential file.
//
// Options is always passed to Run by pointer: Run redirects where the run
// seam writes (to a filter buffer or a line cap) and the injected run closure
// reads o.Stdout dynamically at call time — that only works if Run and the
// closure share the same Options.
type Options struct {
	Cmd, Arg   string
	Filter     string
	Projection string
	Sort       string
	Pipeline   string
	Limit      int
	Prod       bool // read production: MONGO_URL_PRD / MYSQL_URL_PRD
	Stdout     io.Writer
	Stderr     io.Writer
	Env        func(string) string
	EnvPath    string
	run        func(name string, args []string, extraEnv []string, stdin string) error
}

// SetRun injects the process-spawning seam. Exported so the cobra command
// (internal/cli) can wire the real os/exec runner without this package
// exposing the run field directly; unit tests in this package set it too.
func (o *Options) SetRun(run func(name string, args []string, extraEnv []string, stdin string) error) {
	o.run = run
}

// LoadCreds resolves DB credentials: live env first, else the workspace
// credential file <workspace>/.zenify/.env at envPath. Returns a map of the
// keys that were found (MONGO_URL, MYSQL_URL).
// Values are NEVER logged. A missing or unreadable file is not an error here —
// the caller decides whether the resulting absence of MONGO_URL is fatal.
func LoadCreds(getenv func(string) string, envPath string) (map[string]string, error) {
	keys := []string{"MONGO_URL", "MYSQL_URL", "MONGO_URL_PRD", "MYSQL_URL_PRD"}
	var f *devenv.File
	if envPath != "" {
		f, _ = devenv.Load(envPath) // unreadable → treat as absent
	}
	lookup := devenv.Lookup(getenv, f)
	out := map[string]string{}
	for _, k := range keys {
		if v := lookup(k); v != "" {
			out[k] = v
		}
	}
	return out, nil
}

// mysqlConn is MYSQL_URL split into the mysql CLI's parameters.
type mysqlConn struct {
	host, port, user, password, database string
}

// mysqlURLShape is the shape named in every MYSQL_URL error; the value itself
// is never echoed.
const mysqlURLShape = "mysql://user:pass@host:port/database"

// parseMySQLURL splits raw (FR-12.2). User and password come back
// percent-decoded; a missing port is 3306. The returned error never contains
// raw or any part of it — url.Parse's own error does, so it is dropped.
func parseMySQLURL(raw string) (mysqlConn, error) {
	bad := fmt.Errorf("zenify db-read: MYSQL_URL is not of the form %s", mysqlURLShape)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "mysql" || u.Hostname() == "" || u.User == nil || u.User.Username() == "" {
		return mysqlConn{}, bad
	}
	db := strings.TrimPrefix(u.Path, "/")
	if db == "" || strings.Contains(db, "/") {
		return mysqlConn{}, bad
	}
	c := mysqlConn{host: u.Hostname(), port: u.Port(), user: u.User.Username(), database: db}
	c.password, _ = u.User.Password()
	if c.port == "" {
		c.port = "3306"
	}
	return c, nil
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

// MaxOutputBytes caps the stdout bytes of every read, mongo and sql alike.
const MaxOutputBytes = 1 << 20

// byteNotice is the stderr line printed when a read is cut at MaxOutputBytes.
const byteNotice = "… đã cắt ở %d byte (giới hạn output)\n" //znf:allow-lang

// capWriter forwards at most limit lines (limit < 0: no line cap) and at most
// maxBytes bytes to w. It holds back a partial line until its line break, so
// the byte cap cuts on a line boundary; only a single line longer than the
// whole budget is cut mid-line. The first byte past either cap marks it
// truncated and fails the write with errCapReached (a short write), which
// stops the child's output copy. Call Flush after the child returns.
type capWriter struct {
	w         io.Writer
	left      int
	max       int
	bytesLeft int
	pending   []byte
	lineCut   bool
	byteCut   bool
}

func newCapWriter(w io.Writer, limit, maxBytes int) *capWriter {
	return &capWriter{w: w, left: limit, max: maxBytes, bytesLeft: maxBytes}
}

func (c *capWriter) Write(p []byte) (int, error) {
	n := len(p)
	if c.Truncated() {
		return 0, errCapReached
	}
	for len(p) > 0 {
		if c.left == 0 {
			c.lineCut = true
			return n - len(p), errCapReached
		}
		seg, complete := p, false
		if i := bytes.IndexByte(p, '\n'); i >= 0 {
			seg, complete = p[:i+1], true
		}
		if len(c.pending)+len(seg) > c.bytesLeft {
			c.byteCut = true
			if c.bytesLeft == c.max { // one line longer than the whole budget: cut it
				line := append(c.pending, seg...)
				c.pending = nil
				if _, err := c.w.Write(line[:c.bytesLeft]); err != nil {
					return n - len(p), err
				}
			}
			c.pending = nil
			return n - len(p), errCapReached
		}
		if !complete {
			c.pending = append(c.pending, seg...)
			return n, nil
		}
		line := append(c.pending, seg...)
		c.pending = c.pending[:0]
		if _, err := c.w.Write(line); err != nil {
			return n - len(p), err
		}
		c.bytesLeft -= len(line)
		if c.left > 0 {
			c.left--
		}
		p = p[len(seg):]
	}
	return n, nil
}

// Flush writes a held-back final line that had no line break.
func (c *capWriter) Flush() error {
	if len(c.pending) == 0 || c.Truncated() {
		return nil
	}
	_, err := c.w.Write(c.pending)
	c.pending = nil
	return err
}

// Truncated reports whether output arrived past either cap.
func (c *capWriter) Truncated() bool { return c.lineCut || c.byteCut }

// BytesCut reports whether the byte cap, not the line cap, cut the output.
func (c *capWriter) BytesCut() bool { return c.byteCut }

// finish flushes cw and, when it was cut, prints the matching notice. A cut
// is a success.
func finish(cw *capWriter, stderr io.Writer, limit int) error {
	if !cw.Truncated() {
		return cw.Flush()
	}
	if cw.BytesCut() {
		_, _ = fmt.Fprintf(stderr, byteNotice, MaxOutputBytes)
	} else {
		_, _ = fmt.Fprintf(stderr, truncNotice, limit, MaxLimit)
	}
	return nil
}

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
		"  output cut at --limit or 1 MiB prints a notice on stderr; --pipeline never combines with --filter/--projection/--sort",
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
	creds, _ := LoadCreds(o.Env, o.EnvPath)
	mk, sk := "MONGO_URL", "MYSQL_URL"
	if o.Prod {
		mk, sk = mk+"_PRD", sk+"_PRD"
	}
	mongoURL := creds[mk]
	if mongoURL == "" {
		if o.EnvPath == "" {
			_, _ = fmt.Fprintln(o.Stderr, "zenify db-read: chưa có workspace — chạy `zenify up` trước") //znf:allow-lang
			return fmt.Errorf("zenify db-read: no workspace")
		}
		_, _ = fmt.Fprintf(o.Stderr, "zenify db-read: $%s is not set and %s did not supply it.\n", mk, o.EnvPath)
		_, _ = fmt.Fprintln(o.Stderr, "zenify db-read: put "+mk+" there with: zenify env set "+mk)
		return fmt.Errorf("zenify db-read: %s not set", mk)
	}

	if o.Prod {
		key := sk
		if script != "" {
			key = mk
		}
		_, _ = fmt.Fprintf(o.Stderr, "zenify db-read: production (%s)\n", key)
	}

	stdout := o.Stdout
	defer func() { o.Stdout = stdout }()
	if script != "" {
		// Mongo caps rows in the script (--limit); here only bytes are capped.
		cw := newCapWriter(stdout, -1, MaxOutputBytes)
		o.Stdout = cw
		err := o.run("mongosh", []string{"--nodb", "--quiet", "--eval", script}, []string{MongoURLEnv + "=" + mongoURL}, "")
		if err != nil && !cw.Truncated() {
			if o.Prod {
				_, _ = fmt.Fprintf(o.Stderr, "zenify db-read: kiểm lại %s trong .zenify/.env\n", mk) //znf:allow-lang
				return err
			}
			_, _ = fmt.Fprintln(o.Stderr, "zenify db-read: mongosh failed. Separate network from credential before theorising:")
			_, _ = fmt.Fprintf(o.Stderr, "zenify db-read:   nc -z %s 27017   # needs no credential\n", mongoHost)
			_, _ = fmt.Fprintln(o.Stderr, "zenify db-read: port open  -> auth, or a stale $MONGO_URL in .zenify/.env")
			_, _ = fmt.Fprintln(o.Stderr, "zenify db-read: port shut  -> network. This host is reachable directly, no VPN.")
			return err
		}
		return finish(cw, o.Stderr, limit)
	}

	if creds[sk] == "" {
		_, _ = fmt.Fprintf(o.Stderr, "zenify db-read: $%s is not set — same source as $%s: zenify env set %s\n", sk, mk, sk)
		return fmt.Errorf("zenify db-read: %s not set, want %s", sk, mysqlURLShape)
	}
	conn, err := parseMySQLURL(creds[sk])
	if err != nil {
		_, _ = fmt.Fprintln(o.Stderr, err.Error()+" (percent-encode @ : / # ? in user and password)")
		return err
	}
	mysqlRun := func() error {
		return o.run("mysql", []string{
			"--binary-mode", "--quick", "--connect-timeout=10",
			"-h", conn.host, "-P", conn.port, "-u", conn.user,
			"-N", "-B", "-e", sqlStmt, conn.database,
		}, []string{"MYSQL_PWD=" + conn.password}, "")
	}

	if o.Cmd == "tables" {
		// Not row-capped: the substring filter must see every table.
		var buf bytes.Buffer
		o.Stdout = &buf
		if err := mysqlRun(); err != nil {
			return err
		}
		cw := newCapWriter(stdout, -1, MaxOutputBytes)
		_, _ = io.WriteString(cw, filterLines(buf.String(), o.Arg))
		return finish(cw, o.Stderr, limit)
	}
	cw := newCapWriter(stdout, limit, MaxOutputBytes)
	o.Stdout = cw
	// Once a cap is hit the runner kills mysql, so its error is expected:
	// truncation is a success.
	if err := mysqlRun(); err != nil && !cw.Truncated() {
		return err
	}
	return finish(cw, o.Stderr, limit)
}
