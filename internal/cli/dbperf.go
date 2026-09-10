package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/dbperf"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/spf13/cobra"
)

// runDbPerf is the testable core. FAIL-OPEN: always returns nil; any problem
// becomes a printed note. It scans a diff statically; the dynamic explain layer
// runs in the znf:explain-plan skill, not here.
func runDbPerf(diffText string, cfg dbperf.Config, asJSON bool, stdout, stderr io.Writer) error {
	res := dbperf.ScanStatic(dbperf.AddedLines(diffText), cfg)
	if res.SitesScanned == 0 {
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
	fmt.Fprintf(stdout, "## DB-Perf\n%d query site(s) quét tĩnh:\n", res.SitesScanned) //znf:allow-lang
	for _, f := range res.Findings {
		if f.Tier == dbperf.Blocking {
			nBlock++
		}
		fmt.Fprintf(stdout, "  [%s] %s %s:%d %s — %s\n", f.Tier, f.Signal, f.File, f.Line, f.Collection, f.Hint)
	}
	if nBlock > 0 {
		fmt.Fprintf(stdout, "\n%d finding BLOCKING — phải xử lý hoặc waive trước khi ship.\n", nBlock) //znf:allow-lang
	} else {
		fmt.Fprintln(stdout, "\nKhông có BLOCKING (chỉ advisory) — không chặn.") //znf:allow-lang
	}
	return nil
}

func newDbPerfCmd() *cobra.Command {
	var from, to, diffFile string
	var asJSON bool
	c := &cobra.Command{
		Use:   "db-perf",
		Short: "Statically scan a diff for query anti-patterns (two-tier) — advisory core, fail-open",
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
