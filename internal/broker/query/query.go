// Package query serves POST /query: the dbread guards and builders run on the
// broker, with the broker's own credentials.
package query

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/perlogin"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/dbread"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
)

const (
	maxBody        = 64 << 10
	defaultTimeout = 60 * time.Second
	// waitDelay bounds how long a killed child's pipes are waited on, so a
	// grandchild holding stdout/stderr cannot outlive the deadline.
	waitDelay = time.Second
	// statusClientClosed is audited when the client went away before the
	// child finished (nginx's 499); no body is written.
	statusClientClosed = 499
)

// Creds are the broker's database credentials. MainDB / AuditDB name the Mongo
// database used for target main / audit.
type Creds struct {
	MongoStg, MongoPrd, MySQLStg, MySQLPrd, AuditMongoPrd, MainDB, AuditDB string
}

// Handler is the POST /query handler. It must sit behind auth.Require.
type Handler struct {
	Creds         Creds
	Audit         *audit.Logger
	Run           func(ctx context.Context, name string, args, extraEnv []string, stdout, stderr io.Writer) error
	MaxConcurrent int           // per login
	Timeout       time.Duration // 0 = 60s

	slots perlogin.Slots
}

// Validate reports wiring mistakes that would otherwise fail every request
// (MaxConcurrent < 1, a missing credential) or panic (nil Audit or Run).
func (h *Handler) Validate() error {
	var errs []error
	if h.Audit == nil {
		errs = append(errs, errors.New("query: Audit is nil"))
	}
	if h.Run == nil {
		errs = append(errs, errors.New("query: Run is nil"))
	}
	if h.MaxConcurrent < 1 {
		errs = append(errs, errors.New("query: MaxConcurrent must be at least 1"))
	}
	c := h.Creds
	for name, v := range map[string]string{"MongoStg": c.MongoStg, "MongoPrd": c.MongoPrd, "MySQLStg": c.MySQLStg,
		"MySQLPrd": c.MySQLPrd, "AuditMongoPrd": c.AuditMongoPrd, "MainDB": c.MainDB, "AuditDB": c.AuditDB} {
		if v == "" {
			errs = append(errs, fmt.Errorf("query: Creds.%s is empty", name))
		}
	}
	return errors.Join(errs...)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	login, role := auth.Login(r.Context()), auth.Role(r.Context())
	var req api.QueryRequest
	status, rows, nbytes := http.StatusOK, 0, 0
	defer func() {
		h.Audit.Log(audit.Entry{TS: start, Login: login, Role: role, Endpoint: "/query", Env: req.Env, Op: req.Op,
			Args: req, Rows: rows, Bytes: nbytes, MS: time.Since(start).Milliseconds(), Status: status})
	}()
	fail := func(st int, code, msg string) {
		status = st
		auth.WriteError(w, st, code, msg)
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
		fail(http.StatusBadRequest, api.CodeInvalidQuery, "body must be a JSON query object")
		return
	}
	// Before every guard: CheckSQL refuses comments, which would turn a dev's
	// comment-prefixed read into 400 instead of lead_only.
	if role != api.RoleLead && req.Env == "prd" && !devAllowed(req) {
		fail(http.StatusForbidden, api.CodeLeadOnly, "production data reads are lead-only; dev may use explain/indexes/stats/count")
		return
	}
	o, code, msg := h.options(req)
	if code != "" {
		st := http.StatusBadRequest
		if code == api.CodeBrokerMisconfigured {
			st = http.StatusInternalServerError
		}
		fail(st, code, msg)
		return
	}
	if !h.slots.Acquire(login, h.MaxConcurrent) {
		fail(http.StatusTooManyRequests, api.CodeTooManyConcurrent, "too many concurrent queries for this login")
		return
	}
	defer h.slots.Release(login)

	timeout := h.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	o.Stdout = &stdout
	mw := devenv.NewMaskWriter(&stderr, h.secrets())
	o.Stderr = mw
	o.SetRun(func(name string, args, extraEnv []string, _ string) error {
		return h.Run(ctx, name, args, extraEnv, o.Stdout, o.Stderr)
	})
	err := dbread.Run(o)
	_ = mw.Close()
	rows, nbytes = o.Rows, stdout.Len()
	switch {
	case dbread.IsBadArgs(err):
		fail(http.StatusBadRequest, api.CodeInvalidQuery, strings.TrimPrefix(err.Error(), "zenify db-read: "))
		return
	case err == nil: // a success that finished near the deadline is still a success
	case r.Context().Err() != nil: // the client went away: nobody reads a body
		status = statusClientClosed
		return
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		fail(http.StatusGatewayTimeout, api.CodeTimeout, "query exceeded the time limit")
		return
	case err != nil:
		fail(http.StatusBadGateway, api.CodeQueryFailed, strings.TrimSpace(stderr.String()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.QueryResponse{Stdout: stdout.String(), Stderr: stderr.String(), Truncated: o.Truncated, Rows: o.Rows})
}

// options validates req's routing fields and maps it to dbread.Options. The
// returned code is non-empty on a refusal.
func (h *Handler) options(req api.QueryRequest) (o *dbread.Options, code, msg string) {
	if req.Env != "stg" && req.Env != "prd" {
		return nil, api.CodeInvalidQuery, "env must be stg or prd"
	}
	sqlOp := req.Op == "sql" || req.Op == "tables"
	switch {
	case req.Engine == "mysql" && !sqlOp, req.Engine == "mongo" && sqlOp,
		req.Engine != "mysql" && req.Engine != "mongo":
		return nil, api.CodeInvalidQuery, "engine must be mongo (collections|doc|count|find|aggregate|explain|indexes|stats) or mysql (sql|tables)"
	}
	c, prd := h.Creds, req.Env == "prd"
	db := c.MainDB
	mongoPrd := c.MongoPrd
	switch req.Target {
	case "", "main":
	case "audit":
		if req.Engine != "mongo" || !prd {
			return nil, api.CodeInvalidTarget, "target=audit needs engine=mongo and env=prd"
		}
		db, mongoPrd = c.AuditDB, c.AuditMongoPrd
	default:
		return nil, api.CodeInvalidTarget, "target must be main or audit"
	}
	env := map[string]string{"MONGO_URL": c.MongoStg, "MYSQL_URL": c.MySQLStg, "MONGO_URL_PRD": mongoPrd, "MYSQL_URL_PRD": c.MySQLPrd}
	// dbread needs the Mongo URL for every op and the MySQL URL for sql/tables.
	mk, sk := "MONGO_URL", "MYSQL_URL"
	if prd {
		mk, sk = mk+"_PRD", sk+"_PRD"
	}
	if env[mk] == "" || (req.Engine == "mysql" && env[sk] == "") {
		return nil, api.CodeBrokerMisconfigured, "the broker has no credential configured for env=" + req.Env + " target=" + req.Target
	}
	arg := req.Coll
	if req.Op == "sql" {
		arg = req.SQL
	}
	return &dbread.Options{Cmd: req.Op, Arg: arg, Filter: req.Filter, Projection: req.Projection, Sort: req.Sort,
		Pipeline: req.Pipeline, Limit: req.Limit, Prod: prd, MongoDB: db,
		Env: func(k string) string { return env[k] }}, "", ""
}

// devMongoOps are the Mongo ops a dev may run on prd: structure and plans,
// never document content (FR-14.2).
var devMongoOps = map[string]bool{"collections": true, "count": true, "explain": true, "indexes": true, "stats": true}

// devAllowed reports whether a dev may send req to prd. It only narrows: the
// dbread guards still run afterwards.
func devAllowed(req api.QueryRequest) bool {
	switch {
	case req.Target != "" && req.Target != "main":
		return false
	case req.Engine == "mongo":
		return devMongoOps[req.Op]
	case req.Engine == "mysql":
		return req.Op == "tables" || (req.Op == "sql" && devSQL(req.SQL))
	}
	return false
}

// devSQL reports whether stmt, after leading whitespace and comments, starts
// with a plan or schema statement: EXPLAIN [ANALYZE] [FORMAT=x] SELECT|WITH,
// DESCRIBE/DESC, or SHOW INDEX|INDEXES|KEYS|COLUMNS|TABLES|CREATE TABLE|TABLE
// STATUS.
func devSQL(stmt string) bool {
	w := leadingWords(stmt, 6)
	at := func(i int) string {
		if i < len(w) {
			return w[i]
		}
		return ""
	}
	switch at(0) {
	case "DESCRIBE", "DESC":
		return true
	case "SHOW":
		switch at(1) {
		case "INDEX", "INDEXES", "KEYS", "COLUMNS", "TABLES":
			return true
		case "CREATE":
			return at(2) == "TABLE"
		case "TABLE":
			return at(2) == "STATUS"
		}
	case "EXPLAIN":
		i := 1
		if at(i) == "ANALYZE" {
			i++
		}
		if at(i) == "FORMAT" && at(i+1) == "=" && at(i+2) != "" {
			i += 3
		}
		return at(i) == "SELECT" || at(i) == "WITH"
	}
	return false
}

// leadingWords returns up to n upper-cased leading tokens of s: identifier
// words and "=", skipping whitespace and /* */, -- and # comments. It stops
// at any other character or an unterminated comment.
func leadingWords(s string, n int) []string {
	var out []string
	for len(out) < n {
		s = strings.TrimLeft(s, " \t\r\n\f\v")
		switch {
		case strings.HasPrefix(s, "/*"):
			end := strings.Index(s[2:], "*/")
			if end < 0 {
				return out
			}
			s = s[2+end+2:]
		case strings.HasPrefix(s, "--"), strings.HasPrefix(s, "#"):
			end := strings.IndexAny(s, "\r\n")
			if end < 0 {
				return out
			}
			s = s[end:]
		case strings.HasPrefix(s, "="):
			out, s = append(out, "="), s[1:]
		default:
			j := 0
			for j < len(s) && (s[j] == '_' || s[j] >= '0' && s[j] <= '9' || (s[j]|0x20) >= 'a' && (s[j]|0x20) <= 'z') {
				j++
			}
			if j == 0 {
				return out
			}
			out, s = append(out, strings.ToUpper(s[:j])), s[j:]
		}
	}
	return out
}

// secrets are the URLs, URL hosts, users and passwords masked out of child
// stderr; user and password both raw (as written in the URL) and decoded.
func (h *Handler) secrets() []string {
	var out []string
	for _, v := range []string{h.Creds.MongoStg, h.Creds.MongoPrd, h.Creds.MySQLStg, h.Creds.MySQLPrd, h.Creds.AuditMongoPrd} {
		if v == "" {
			continue
		}
		out = append(out, v)
		if u, err := url.Parse(v); err == nil && u.Hostname() != "" {
			out = append(out, u.Hostname())
		}
		for _, raw := range userinfo(v) {
			out = append(out, raw)
			if dec, err := url.PathUnescape(raw); err == nil && dec != raw {
				out = append(out, dec)
			}
		}
	}
	return out
}

// userinfo returns the raw user and password of a scheme://user:pass@host URL
// (either may be absent), as written in s.
func userinfo(s string) []string {
	i := strings.Index(s, "://")
	if i < 0 {
		return nil
	}
	auth := s[i+3:]
	if j := strings.IndexAny(auth, "/?#"); j >= 0 {
		auth = auth[:j]
	}
	at := strings.LastIndex(auth, "@")
	if at < 0 {
		return nil
	}
	var out []string
	for _, p := range strings.SplitN(auth[:at], ":", 2) {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// childEnv is the whole environment of a mongosh/mysql child: PATH and HOME
// from getenv plus extra. The broker's own env holds every prod secret, so a
// child must never inherit it.
func childEnv(getenv func(string) string, extra []string) []string {
	var env []string
	for _, k := range []string{"PATH", "HOME"} {
		if v := getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	return append(env, extra...)
}

// ExecRun is the production Run seam: one mongosh/mysql child under ctx. A
// failed stdout write (dbread's line or byte cap) kills the child at once.
func ExecRun(ctx context.Context, name string, args, extraEnv []string, stdout, stderr io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204 -- name is "mongosh" or "mysql" from dbread, args are dbread-built
	c.Env = childEnv(os.Getenv, extraEnv)
	c.Stdout = cancelOnErr{stdout, cancel}
	c.Stderr = stderr
	c.WaitDelay = waitDelay
	return c.Run()
}

type cancelOnErr struct {
	w      io.Writer
	cancel context.CancelFunc
}

func (c cancelOnErr) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if err != nil {
		c.cancel()
	}
	return n, err
}
