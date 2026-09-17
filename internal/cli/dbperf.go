package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/dbperf"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/spf13/cobra"
)

// runDbPerf is the testable core. FAIL-OPEN: always returns nil; any problem
// becomes a printed note. It scans a diff statically; the dynamic explain layer
// runs in the znf:explain-plan skill, not here.
func runDbPerf(diffText string, cfg dbperf.Config, asJSON bool, stdout, stderr io.Writer) error {
	res := dbperf.ScanStatic(dbperf.AddedLines(diffText), cfg)
	if res.SitesScanned == 0 {
		// Plain, never styled: this branch runs BEFORE the asJSON check below, so
		// `db-perf --json` with 0 sites also lands here — ui.Note would put ANSI on
		// stdout on a real TTY with color on, which a --json consumer must never see.
		fmt.Fprintln(stdout, "db-perf: không có query backend trong diff — gate pass") //znf:allow-lang
		return nil
	}
	if asJSON {
		b, err := json.Marshal(res)
		if err != nil {
			fmt.Fprintln(stderr, "db-perf: không phân tích được: marshal lỗi (fail-open)") //znf:allow-lang
			return nil
		}
		fmt.Fprintln(stdout, string(b))
		return nil
	}
	var nBlock int
	u := ui.New(stdout)
	u.Section("DB-Perf")
	u.Note(fmt.Sprintf("%d query site(s) quét tĩnh:", res.SitesScanned)) //znf:allow-lang
	for _, f := range res.Findings {
		st := ui.StatusInfo
		if f.Tier == dbperf.Blocking {
			nBlock++
			st = ui.StatusFail
		}
		u.Step(st, fmt.Sprintf("[%s] %s %s:%d", f.Tier, f.Signal, f.File, f.Line), fmt.Sprintf("%s — %s", f.Collection, f.Hint))
	}
	u.Blank()
	if nBlock > 0 {
		u.Step(ui.StatusFail, fmt.Sprintf("%d finding BLOCKING", nBlock), "phải xử lý hoặc waive trước khi ship.") //znf:allow-lang
	} else {
		u.Note("Không có BLOCKING (chỉ advisory) — không chặn.") //znf:allow-lang
	}
	return nil
}

func newDbPerfCmd() *cobra.Command {
	var from, to, diffFile string
	var asJSON bool
	c := &cobra.Command{
		Use:   "db-perf",
		Short: "Quét tĩnh một diff tìm anti-pattern query (hai tầng) — advisory, fail-open", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := loadDbPerfConfig(cmd.ErrOrStderr())
			diff := ""
			if diffFile != "" {
				b, err := os.ReadFile(diffFile) //nolint:gosec // G304 -- diffFile is the explicit --diff-file flag from the invoking user, not externally-tainted
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "db-perf: không đọc được diff-file (fail-open): %v\n", err) //znf:allow-lang
				} else {
					diff = string(b)
				}
			} else {
				r := gitx.ExecRunner()
				out, err := r.Run(".", "diff", from+".."+to)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "db-perf: không đọc được git diff (fail-open): %v\n", err) //znf:allow-lang
				} else {
					diff = string(out)
				}
			}
			return runDbPerf(diff, cfg, asJSON, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	c.Flags().StringVar(&from, "from", "origin/staging", "base ref for the diff")
	c.Flags().StringVar(&to, "to", "HEAD", "head ref for the diff")
	c.Flags().StringVar(&diffFile, "diff-file", "", "read a unified diff from this file instead of git")
	c.Flags().BoolVar(&asJSON, "json", false, "emit the machine-readable Result JSON")
	return c
}

// loadDbPerfConfig resolves the knowledge-store config; missing → defaults.
func loadDbPerfConfig(stderr io.Writer) dbperf.Config {
	store := resolveDocsStore("", os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir)
	cfg, err := dbperf.Load(filepath.Join(store, ".config", "db-collections.json"))
	if err != nil {
		fmt.Fprintf(stderr, "db-perf: config lỗi, dùng default (fail-open): %v\n", err) //znf:allow-lang
		return dbperf.Defaults()
	}
	return cfg
}
