// Package query serves POST /query: the dbread guards and builders run on the
// broker, with the broker's own credentials.
package query

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/dbread"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
)

const (
	maxBody        = 64 << 10
	defaultTimeout = 60 * time.Second
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

	mu     sync.Mutex
	active map[string]int
}

func (h *Handler) acquire(login string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active == nil {
		h.active = map[string]int{}
	}
	if h.active[login] >= h.MaxConcurrent {
		return false
	}
	h.active[login]++
	return true
}

func (h *Handler) release(login string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active[login]--; h.active[login] <= 0 {
		delete(h.active, login)
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	login := auth.Login(r.Context())
	var req api.QueryRequest
	status, rows, nbytes := http.StatusOK, 0, 0
	defer func() {
		h.Audit.Log(audit.Entry{TS: start, Login: login, Endpoint: "/query", Env: req.Env, Op: req.Op,
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
	o, code, msg := h.options(req)
	if code != "" {
		fail(http.StatusBadRequest, code, msg)
		return
	}
	if !h.acquire(login) {
		fail(http.StatusTooManyRequests, api.CodeTooManyConcurrent, "too many concurrent queries for this login")
		return
	}
	defer h.release(login)

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
		return nil, api.CodeInvalidQuery, "engine must be mongo (collections|doc|count|find|aggregate|explain) or mysql (sql|tables)"
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
	arg := req.Coll
	if req.Op == "sql" {
		arg = req.SQL
	}
	return &dbread.Options{Cmd: req.Op, Arg: arg, Filter: req.Filter, Projection: req.Projection, Sort: req.Sort,
		Pipeline: req.Pipeline, Limit: req.Limit, Prod: prd, MongoDB: db,
		Env: func(k string) string { return env[k] }}, "", ""
}

// secrets are the URLs and URL hosts masked out of child stderr.
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
