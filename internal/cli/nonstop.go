package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/nonstopstats"
	"github.com/spf13/cobra"
)

func newNonstopCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "nonstop",
		Short: "Công cụ phụ trợ cho orchestrator ngày znf:nonstop", //znf:allow-lang
	}
	c.AddCommand(newNonstopStatsCmd())
	return c
}

func newNonstopStatsCmd() *cobra.Command {
	var (
		dir    string
		asJSON bool
		today  string
	)
	c := &cobra.Command{
		Use:   "stats",
		Short: "Thang size đã học và thống kê ngày/tuần từ estimates.md của nonstop", //znf:allow-lang
		Long: "Đọc <dir>/estimates.md (mặc định: <workspace>/.zenify/nonstop) và in thang thời gian\n" + //znf:allow-lang
			"S/M/L đã học, tỉ lệ đúng và điểm của hôm nay và 7 ngày, cùng các luật calibration.\n" + //znf:allow-lang
			"Thiếu estimates.md không phải lỗi: kết quả mang missing=true.", //znf:allow-lang
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if today == "" {
				today = time.Now().Format("2006-01-02")
			} else if _, err := time.Parse("2006-01-02", today); err != nil {
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("--today must be YYYY-MM-DD: %w", err))
			}
			if dir == "" {
				cwd, _ := os.Getwd()
				ws, _, ok := resolveWorkspace(cwd, "", os.Getenv, os.UserHomeDir)
				if !ok {
					return exitcode.New(exitcode.BadArgs, errors.New("no workspace found; pass --dir"))
				}
				dir = filepath.Join(ws, ".zenify", "nonstop")
			}
			return runNonstopStats(cmd.OutOrStdout(), dir, today, asJSON)
		},
	}
	c.Flags().StringVar(&dir, "dir", "", "directory holding estimates.md (default: <workspace>/.zenify/nonstop)")
	c.Flags().BoolVar(&asJSON, "json", false, "output JSON instead of a short table")
	c.Flags().StringVar(&today, "today", "", "the day to report, YYYY-MM-DD (default: local today)")
	return c
}

func runNonstopStats(w io.Writer, dir, today string, asJSON bool) error {
	var res nonstopstats.Result
	b, err := os.ReadFile(filepath.Join(dir, "estimates.md")) //nolint:gosec // G304 -- dir is a CLI flag of a local, read-only helper
	switch {
	case errors.Is(err, os.ErrNotExist):
		res = nonstopstats.Compute(nil, 0, today)
		res.Missing = true
	case err != nil:
		return exitcode.New(exitcode.Fail, err)
	default:
		lines, malformed := nonstopstats.Parse(string(b))
		var d nonstopstats.Durations
		dmal := 0
		db, derr := os.ReadFile(filepath.Join(dir, "durations.log")) //nolint:gosec // G304 -- same CLI-flag dir as estimates.md
		switch {
		case derr == nil:
			d, dmal = nonstopstats.ParseDurations(string(db))
			nonstopstats.ApplyDurations(lines, d)
		case !errors.Is(derr, os.ErrNotExist):
			return exitcode.New(exitcode.Fail, derr)
		}
		res = nonstopstats.Compute(lines, malformed, today)
		res.DurationsMalformed = dmal
	}
	if asJSON {
		out, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return exitcode.New(exitcode.Fail, err)
		}
		_, err = fmt.Fprintf(w, "%s\n", out)
		return err
	}
	renderNonstopStats(w, res)
	return nil
}

func renderNonstopStats(w io.Writer, r nonstopstats.Result) {
	if r.Missing {
		_, _ = fmt.Fprintln(w, "no estimates.md yet")
		return
	}
	s := r.Scale
	_, _ = fmt.Fprintf(w, "scale (%s, %d tasks): S<%dm M<%dm L<%dm\n", s.Source, s.N, s.S, s.M, s.L)
	t := r.Today
	_, _ = fmt.Fprintf(w, "today: %d/%d hit, %d over, %d under, points %d\n", t.Hit, t.Total, t.Over, t.Under, t.Points)
	share := "-"
	if r.Week.ReviewShare != nil {
		share = fmt.Sprintf("%.0f%%", *r.Week.ReviewShare*100)
	}
	_, _ = fmt.Fprintf(w, "review: %dm (%s)\n", t.ReviewMinutes, share)
	_, _ = fmt.Fprintf(w, "tiers 7d: T1 %d · T2 %d · T3 %d\n", r.Tiers.T1, r.Tiers.T2, r.Tiers.T3)
	hit, ppd := "-", "-"
	if r.Week.HitRate != nil {
		hit = fmt.Sprintf("%.0f%%", *r.Week.HitRate*100)
	}
	if r.Week.PointsPerDay != nil {
		ppd = fmt.Sprintf("%.1f", *r.Week.PointsPerDay)
	}
	_, _ = fmt.Fprintf(w, "7 days: hit %s, %s points/day\n", hit, ppd)
	for _, rule := range r.Rules {
		_, _ = fmt.Fprintf(w, "rule: %s\n", rule)
	}
	if r.Malformed > 0 {
		_, _ = fmt.Fprintf(w, "malformed lines: %d\n", r.Malformed)
	}
	if r.DurationsMalformed > 0 {
		_, _ = fmt.Fprintf(w, "durations.log malformed lines: %d\n", r.DurationsMalformed)
	}
}
