package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/brokerclient"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/dbread"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/spf13/cobra"
)

// dbReadTimeout bounds one mongosh/mysql child, on top of the server-side
// query timeouts the builders set.
const dbReadTimeout = 60 * time.Second

// brokerCallTimeout bounds one db-read/redis-read broker call: the broker's own
// query timeout plus room for gh and the token exchange.
const brokerCallTimeout = dbReadTimeout + 15*time.Second

var dbReadLong = strings.Join([]string{
	"Đọc chỉ-đọc các database dùng chung của zenify. Không lệnh nào nhận code tự do.", //znf:allow-lang
	"",
	"  collections [chuỗi]        liệt kê collection Mongo, lọc theo chuỗi con", //znf:allow-lang
	"  tables [chuỗi]             liệt kê bảng MySQL, lọc theo chuỗi con",       //znf:allow-lang
	"  doc <collection>           in một document để xem tên field",             //znf:allow-lang
	"  count <collection>         đếm document (--filter để đếm có điều kiện)",  //znf:allow-lang
	"  find <collection>          --filter --projection --sort --limit",
	"  aggregate <collection>     --pipeline (từ chối $out/$merge) --limit",                                    //znf:allow-lang
	"  explain <collection>       executionStats, không áp --limit (chạy truy vấn đọc, tối đa 30 s)",           //znf:allow-lang
	"  indexes <collection>       in getIndexes() của collection",                                              //znf:allow-lang
	"  stats <collection>         kích thước, số document, số index của collection",                            //znf:allow-lang
	"  sql '<câu lệnh>'           một câu SELECT/WITH/SHOW/DESCRIBE/EXPLAIN, chạy trong transaction READ ONLY", //znf:allow-lang
	"",
	"--limit mặc định 50, tối đa 200; stdout tối đa 1 MiB; kết quả bị cắt thì in thông báo ra stderr. Thay cho distinct:", //znf:allow-lang
	`  zenify db-read aggregate <collection> --pipeline '[{"$group":{"_id":"$field"}}]'`,
}, "\n")

// newDBReadCmd builds `zenify db-read <collections|tables|doc|count|find|aggregate|explain|indexes|stats|sql> [arg]`
// — the portable port of ~/.local/bin/db_read (FR-033).
func newDBReadCmd() *cobra.Command {
	var filter, projection, sort, pipeline, manifestPath, overlayPath, target string
	var limit int
	var prd bool
	cmd := &cobra.Command{
		Use:           "db-read <collections|tables|doc|count|find|aggregate|explain|indexes|stats|sql> [arg]",
		Short:         "Đọc chỉ-đọc các database dùng chung của zenify (không bao giờ ghi, không in secret)", //znf:allow-lang
		Long:          dbReadLong,
		Args:          cobra.RangeArgs(1, 2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := ""
			if len(args) > 1 {
				arg = args[1]
			}
			o := &dbread.Options{
				Cmd:        args[0],
				Arg:        arg,
				Filter:     filter,
				Projection: projection,
				Sort:       sort,
				Pipeline:   pipeline,
				Limit:      limit,
				Prod:       prd,
				Stdout:     cmd.OutOrStdout(),
				Stderr:     cmd.ErrOrStderr(),
				Env:        os.Getenv,
				EnvPath:    workspaceEnvPath(),
			}
			bc, ok, err := brokerFromFlags(manifestPath, overlayPath)
			if err != nil {
				return err // a missing or invalid manifest must not fall back to local credentials
			}
			if ok {
				return runDBReadBroker(cmd, bc, o, target)
			}
			if target != "" && target != "main" {
				return exitcode.New(exitcode.BadArgs, errors.New("zenify db-read: --target "+target+" cần broker")) //znf:allow-lang
			}
			o.SetRun(func(name string, args []string, extraEnv []string, _ string) error {
				stderr := o.Stderr
				if prd { // a driver error can name the production URL or its hosts
					mw := devenv.NewMaskWriter(stderr, prodSecrets(o.Env, o.EnvPath))
					defer func() { _ = mw.Close() }()
					stderr = mw
				}
				return runDBChild(cmd.Context(), name, args, extraEnv, o.Stdout, stderr)
			})
			return dbread.Run(o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&filter, "filter", "", "JSON object filter (find, count, explain)")
	f.StringVar(&projection, "projection", "", "JSON object projection (find, explain)")
	f.StringVar(&sort, "sort", "", "JSON object sort (find, explain)")
	f.StringVar(&pipeline, "pipeline", "", "JSON array pipeline (aggregate, explain)")
	f.IntVar(&limit, "limit", dbread.DefaultLimit, "max documents or rows, 1..200")
	f.StringVar(&manifestPath, "manifest", "", brokerManifestUsage)
	f.StringVar(&overlayPath, "overlay", "", brokerOverlayUsage)
	f.StringVar(&target, "target", "", "database: main (default), audit (Mongo, prd only) or lumi (lumi-agent MySQL)")
	f.BoolVar(&prd, "prd", false, "read production: via the broker; without one: MONGO_URL_PRD / MYSQL_URL_PRD")
	return cmd
}

// runDBReadBroker validates o exactly as dbread.Run would, then sends it to
// the broker and prints the reply. A 400 is a refused request (BadArgs).
func runDBReadBroker(cmd *cobra.Command, bc *brokerclient.Client, o *dbread.Options, target string) error {
	limit, err := dbread.Validate(o)
	if err != nil {
		return err
	}
	req := api.QueryRequest{Env: "stg", Engine: "mongo", Op: o.Cmd, Coll: o.Arg, Filter: o.Filter,
		Projection: o.Projection, Sort: o.Sort, Pipeline: o.Pipeline, Limit: limit, Target: target}
	if o.Prod {
		req.Env = "prd"
		_, _ = io.WriteString(o.Stderr, "zenify db-read: production (via broker)\n")
	}
	switch o.Cmd {
	case "tables": // Coll carries the optional substring filter
		req.Engine = "mysql"
	case "sql":
		req.Engine, req.Coll, req.SQL = "mysql", "", o.Arg
	}
	if err := devPrdPrecheck(bc, req.Env, api.DevQueryAllowed(req)); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), brokerCallTimeout)
	defer cancel()
	var resp api.QueryResponse
	if err := bc.Post(ctx, "/query", req, &resp); err != nil {
		return brokerCallErr(err)
	}
	_, _ = io.WriteString(o.Stdout, resp.Stdout)
	_, _ = io.WriteString(o.Stderr, resp.Stderr)
	return nil
}

// leadOnlyMsg is FR-14.6's answer to a dev hitting a lead-only request.
func leadOnlyMsg() string {
	return "lệnh này chỉ dành cho lead (team leaders); dev trên prd chỉ dùng " + api.DevPrdSummary() //znf:allow-lang
}

// devPrdPrecheck refuses, without calling the broker, a prd request the
// cached role (dev) is not allowed to make; the broker enforces the same
// lists. No cached role: the broker decides.
func devPrdPrecheck(bc *brokerclient.Client, env string, allowed bool) error {
	if env == "prd" && !allowed && bc.CachedRole() == api.RoleDev {
		return errors.New("zenify: " + leadOnlyMsg())
	}
	return nil
}

// brokerCallErr maps a broker 400 to BadArgs and 403 lead_only to leadOnlyMsg;
// anything else passes through.
func brokerCallErr(err error) error {
	var ae *brokerclient.APIError
	switch {
	case errors.As(err, &ae) && ae.Code == api.CodeLeadOnly:
		return errors.New("zenify: " + leadOnlyMsg())
	case errors.As(err, &ae) && ae.Status == http.StatusBadRequest:
		return exitcode.New(exitcode.BadArgs, err)
	}
	return err
}

// prodSecrets is every production DB URL db-read may use, plus its hosts.
func prodSecrets(getenv func(string) string, envPath string) []string {
	creds, _ := dbread.LoadCreds(getenv, envPath)
	var out []string
	for _, k := range []string{"MONGO_URL_PRD", "MYSQL_URL_PRD"} {
		if v := creds[k]; v != "" {
			out = append(append(out, v), urlHosts(v)...)
		}
	}
	return out
}

// runDBChild runs one mongosh/mysql child under dbReadTimeout. A failed stdout
// write (dbread's line or byte cap refusing more output) kills the child at once, so
// the rest of a large result is never transferred.
func runDBChild(parent context.Context, name string, args, extraEnv []string, stdout, stderr io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, dbReadTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204 -- name is always "mongosh" or "mysql" from this package's own callers, args are internally-computed subcommands, not attacker-controlled shell input
	c.Env = append(os.Environ(), extraEnv...)
	c.Stdout = cancelOnErr{stdout, cancel}
	c.Stderr = stderr
	return c.Run()
}

// cancelOnErr cancels the child's context on the first failed write.
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
