package cli

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/dbread"
	"github.com/spf13/cobra"
)

// dbReadTimeout bounds one mongosh/mysql child, on top of the server-side
// query timeouts the builders set.
const dbReadTimeout = 60 * time.Second

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
	"  sql '<câu lệnh>'           một câu SELECT/WITH/SHOW/DESCRIBE/EXPLAIN, chạy trong transaction READ ONLY", //znf:allow-lang
	"",
	"--limit mặc định 50, tối đa 200; kết quả bị cắt thì in thông báo ra stderr. Thay cho distinct:", //znf:allow-lang
	`  zenify db-read aggregate <collection> --pipeline '[{"$group":{"_id":"$field"}}]'`,
}, "\n")

// newDBReadCmd builds `zenify db-read <collections|tables|doc|count|find|aggregate|explain|sql> [arg]`
// — the portable port of ~/.local/bin/db_read (FR-033).
func newDBReadCmd() *cobra.Command {
	var filter, projection, sort, pipeline string
	var limit int
	cmd := &cobra.Command{
		Use:           "db-read <collections|tables|doc|count|find|aggregate|explain|sql> [arg]",
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
				Cmd:          args[0],
				Arg:          arg,
				Filter:       filter,
				Projection:   projection,
				Sort:         sort,
				Pipeline:     pipeline,
				Limit:        limit,
				Stdout:       cmd.OutOrStdout(),
				Stderr:       cmd.ErrOrStderr(),
				Env:          os.Getenv,
				SettingsPath: workspaceSettingsPath(),
			}
			o.SetRun(func(name string, args []string, extraEnv []string, _ string) error {
				return runDBChild(cmd.Context(), name, args, extraEnv, o.Stdout, o.Stderr)
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
	return cmd
}

// runDBChild runs one mongosh/mysql child under dbReadTimeout. A failed stdout
// write (dbread's line cap refusing more output) kills the child at once, so
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
