package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/routelog"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/spf13/cobra"
)

func defaultRouteLogDir() (string, error) { return znfStoreDir(gitCommonDirRun, "route-log") }

// runRouteLogRecord: stdin = Record JSON → store. Best-effort, always nil (never blocks a skill).
func runRouteLogRecord(stdin io.Reader, stderr io.Writer, dirFn func() (string, error)) error {
	data, err := io.ReadAll(stdin)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	var r routelog.Record
	if err := json.Unmarshal(data, &r); err != nil {
		fmt.Fprintln(stderr, "route-log record: JSON hỏng, bỏ qua:", err) //znf:allow-lang
		return nil
	}
	dir, err := dirFn()
	if err != nil {
		fmt.Fprintln(stderr, "route-log record: không resolve store, bỏ qua:", err) //znf:allow-lang
		return nil
	}
	if _, err := routelog.WriteRecord(dir, r); err != nil {
		fmt.Fprintln(stderr, "route-log record: ghi lỗi, bỏ qua:", err) //znf:allow-lang
	}
	return nil
}

// runRouteLogRecordPlan reads a plan file, counts tasks and distinct files, and writes a
// site=plan-metrics record for the current branch. Best-effort like record.
func runRouteLogRecordPlan(planPath string, fanin int, stderr io.Writer, dirFn func() (string, error), git func(...string) ([]byte, error)) error {
	if planPath == "" {
		fmt.Fprintln(stderr, "route-log record-plan: thiếu --plan, bỏ qua") //znf:allow-lang
		return nil
	}
	b, err := os.ReadFile(planPath) //nolint:gosec // G304 -- caller-supplied plan path
	if err != nil {
		fmt.Fprintln(stderr, "route-log record-plan: không đọc được plan, bỏ qua:", err) //znf:allow-lang
		return nil
	}
	tasks, files := routelog.PlanMetrics(b)
	branch := ""
	if out, err := git("branch", "--show-current"); err == nil {
		branch = strings.TrimSpace(string(out))
	}
	repo := ""
	if out, err := git("rev-parse", "--show-toplevel"); err == nil {
		p := strings.TrimSpace(string(out))
		repo = p[strings.LastIndex(p, "/")+1:]
	}
	r := routelog.Record{
		TS: time.Now().UTC().Format(time.RFC3339), Repo: repo, Branch: branch, Site: "plan-metrics",
		Model: "none", Strong: strongFromEnv(), Tasks: tasks, FilesPred: files, Fanin: fanin,
	}
	dir, err := dirFn()
	if err != nil {
		fmt.Fprintln(stderr, "route-log record-plan: không resolve store, bỏ qua:", err) //znf:allow-lang
		return nil
	}
	if _, err := routelog.WriteRecord(dir, r); err != nil {
		fmt.Fprintln(stderr, "route-log record-plan: ghi lỗi, bỏ qua:", err) //znf:allow-lang
	}
	return nil
}

func strongFromEnv() string {
	switch v := os.Getenv("ZNF_STRONG_MODEL"); v {
	case "fable", "opus":
		return v
	}
	return "opus"
}

func gitOut(args ...string) ([]byte, error) {
	return exec.Command("git", args...).Output() //nolint:gosec // G204 -- fixed git binary, internal args
}

// runRouteLogShow prints the calibration summary; --json prints the records; --since filters by TS.
func runRouteLogShow(stdout, stderr io.Writer, asJSON bool, since string, dirFn func() (string, error)) error {
	none := func() error { ui.New(stdout).Note("no routes logged yet"); return nil }
	dir, err := dirFn()
	if err != nil {
		return none()
	}
	recs, err := routelog.LoadRecords(dir)
	if err != nil {
		return none()
	}
	if since != "" {
		cut, err := parseSince(since, time.Now())
		if err != nil {
			return err
		}
		kept := recs[:0]
		for _, r := range recs {
			if ts, err := time.Parse(time.RFC3339, r.TS); err == nil && !ts.Before(cut) {
				kept = append(kept, r)
			}
		}
		recs = kept
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(recs)
	}
	if len(recs) == 0 {
		return none()
	}
	s := routelog.Summarize(recs)
	u := ui.New(stdout)
	f := u.Flow("zenify route-log")
	f.Group(ui.MarkerDone, "Summary")
	f.Line(ui.StatusInfo, "routes", fmt.Sprintf("%d", s.Total))
	sites := make([]string, 0, len(s.BySite))
	for k := range s.BySite {
		sites = append(sites, k)
	}
	sort.Strings(sites)
	for _, site := range sites {
		models := s.ModelBySite[site]
		keys := make([]string, 0, len(models))
		for k := range models {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", k, models[k]))
		}
		f.Line(ui.StatusInfo, site, fmt.Sprintf("%d (%s)", s.BySite[site], strings.Join(parts, " ")))
	}
	f.Line(ui.StatusInfo, "strong model sent", fmt.Sprintf("%d", s.StrongSent))
	gates := make([]string, 0, len(s.GateFires))
	for k := range s.GateFires {
		gates = append(gates, k)
	}
	sort.Strings(gates)
	gp := make([]string, 0, len(gates))
	for _, g := range gates {
		gp = append(gp, fmt.Sprintf("%s(%d)", g, s.GateFires[g]))
	}
	f.Line(ui.StatusInfo, "gates fired", strings.Join(gp, " "))
	if s.ArchitectDecided > 0 {
		st := ui.StatusOK
		if s.ArchitectChanged*100 < 40*s.ArchitectDecided {
			st = ui.StatusWarn
		}
		f.Line(st, "architect changed_decision", fmt.Sprintf("%d/%d (%.0f%%)", s.ArchitectChanged, s.ArchitectDecided, float64(s.ArchitectChanged)*100/float64(s.ArchitectDecided)))
	}
	mt := ui.StatusOK
	if s.ManualNoTrigger > 0 {
		mt = ui.StatusWarn
	}
	f.Line(mt, "manual without trigger", fmt.Sprintf("%d", s.ManualNoTrigger))
	f.Close(fmt.Sprintf("%d route(s) logged", s.Total))
	return nil
}

func newRouteLogCmd() *cobra.Command {
	var asJSON bool
	var since string
	c := &cobra.Command{
		Use:   "route-log",
		Short: "Xem log calibrate của select-route: model đã gửi theo site, gate nào fire, architect đổi quyết định bao nhiêu (--json in record)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRouteLogShow(cmd.OutOrStdout(), cmd.ErrOrStderr(), asJSON, since, defaultRouteLogDir)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "in toàn bộ record JSON")                               //znf:allow-lang
	c.Flags().StringVar(&since, "since", "", "chỉ lấy record từ mốc: <n>h|<n>d|<n>w hoặc YYYY-MM-DD") //znf:allow-lang
	record := &cobra.Command{
		Use: "record", Hidden: true, Args: cobra.NoArgs,
		Short: "Ghi một route record (JSON qua stdin), best-effort", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRouteLogRecord(cmd.InOrStdin(), cmd.ErrOrStderr(), defaultRouteLogDir)
		},
	}
	var planPath string
	var fanin int
	recordPlan := &cobra.Command{
		Use: "record-plan", Hidden: true, Args: cobra.NoArgs,
		Short: "Đếm task và file trong plan, ghi record site=plan-metrics", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRouteLogRecordPlan(planPath, fanin, cmd.ErrOrStderr(), defaultRouteLogDir, gitOut)
		},
	}
	recordPlan.Flags().StringVar(&planPath, "plan", "", "path plan")                          //znf:allow-lang
	recordPlan.Flags().IntVar(&fanin, "fanin", 0, "số consumer scout đếm được (0 = chưa có)") //znf:allow-lang
	c.AddCommand(record, recordPlan)
	return c
}
