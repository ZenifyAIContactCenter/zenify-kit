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
	// devPrd: plan and schema only, explain without execution (FR-14.2).
	devPrd := role != api.RoleLead && req.Env == "prd"
	if devPrd && !api.DevQueryAllowed(req) {
		msg := "production data reads are lead-only; dev may use explain/indexes/stats/count"
		if req.Engine == "mongo" && req.Op == "count" {
			msg = "a filtered count on production is lead-only; dev gets the collection total (estimatedDocumentCount) by omitting the filter"
		}
		fail(http.StatusForbidden, api.CodeLeadOnly, msg)
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
	o.PlanOnly = devPrd

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
