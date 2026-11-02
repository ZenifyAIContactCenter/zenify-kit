package cli

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
	"github.com/spf13/cobra"
)

const trackSep = " · "

// trackMerged asks whether a PR is merged; a test replaces it at the gh boundary.
var trackMerged trackstats.MergedFunc = trackstats.GhMerged

// trackPRTitle fetches a PR title; a test replaces it at the gh boundary.
var trackPRTitle = trackstats.GhPRTitle

// mergedRecheck is how long a gh merged check stays fresh before stats asks again.
const mergedRecheck = 5 * time.Minute

var (
	trackIDRe     = regexp.MustCompile(`^(cook|fix|hotfix)-\d{6}-[0-9a-f]{4}$`)
	trackKinds    = []string{"cook", "fix", "hotfix"}
	trackSteps    = []string{"lint", "build", "contract", "behaviour", "review"}
	trackFindings = regexp.MustCompile(`^\d+,\d+,\d+,\d+$`)
	trackTasksRe  = regexp.MustCompile(`^Task \d+: complete`)
)

func newTrackCmd() *cobra.Command {
	var ws string
	c := &cobra.Command{
		Use:   "track",
		Short: "Ghi và báo cáo các lượt cook/fix/hotfix cho pane dev track", //znf:allow-lang
		Long: "Ghi các dòng start/est/ship/fix/ci/end vào <workspace>/.zenify/track/log.md và in số liệu\n" + //znf:allow-lang
			"cho pane. Bật bằng `zenify feature track on`; khi tắt, mọi lệnh ghi in `track: off` và\n" + //znf:allow-lang
			"không ghi gì.", //znf:allow-lang
	}
	c.PersistentFlags().StringVar(&ws, "workspace", "", "workspace root (default: resolved from the current directory)")
	c.AddCommand(
		newTrackStartCmd(&ws), newTrackEstCmd(&ws), newTrackShipCmd(&ws),
		newTrackFixCmd(&ws), newTrackCICmd(&ws), newTrackEndCmd(&ws), newTrackStatsCmd(&ws),
		newTrackBacklogCmd(&ws), newTrackOnboardCmd(&ws),
	)
	return c
}

// trackWorkspace resolves the workspace and whether the track feature is on.
func trackWorkspace(flag string) (string, bool, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, exitcode.New(exitcode.Fail, err)
	}
	ws, _, ok := resolveWorkspace(cwd, flag, os.Getenv, os.UserHomeDir)
	if !ok {
		return "", false, exitcode.New(exitcode.BadArgs, errors.New("no workspace found; pass --workspace"))
	}
	feats, err := manifest.ReadFeatures(filepath.Join(ws, ".zenify-overlay.yaml"))
	if err != nil {
		return "", false, exitcode.New(exitcode.Fail, err)
	}
	return ws, feats["track"], nil
}

func badTrack(format string, a ...any) error {
	return exitcode.New(exitcode.BadArgs, fmt.Errorf(format, a...))
}

// trackRow builds one validated log row for a writing subcommand.
type trackRow func(args []string) (kind string, fields []string, needStarted bool, err error)

// runTrackWrite is the shared flow of every writing subcommand: feature gate,
// validation, started/ended checks, then one append.
func runTrackWrite(w io.Writer, flag string, nargs int, args []string, build trackRow) error {
	ws, on, err := trackWorkspace(flag)
	if err != nil {
		return err
	}
	if !on {
		_, err = fmt.Fprintln(w, "track: off")
		return err
	}
	if len(args) != nargs {
		return badTrack("expected %d argument(s), got %d", nargs, len(args))
	}
	kind, fields, needStarted, err := build(args)
	if err != nil {
		return err
	}
	logPath := filepath.Join(ws, ".zenify", "track", "log.md")
	if needStarted {
		if err := checkTrackID(logPath, args[0], kind == "end"); err != nil {
			return err
		}
	}
	if err := appendTrackRow(logPath, kind, fields); err != nil {
		return err
	}
	if kind == "end" {
		// The hook unbinds the session only on this line, so a failed end keeps the binding.
		_, err = fmt.Fprintf(w, "track: ended %s\n", args[0])
	}
	return err
}

func appendTrackRow(logPath, kind string, fields []string) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	line := time.Now().UTC().Format(time.RFC3339) + trackSep + kind + trackSep + strings.Join(fields, trackSep) + "\n"
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // G302/G304 -- path is computed from the resolved workspace
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	if _, err := f.Write([]byte(line)); err != nil {
		_ = f.Close()
		return exitcode.New(exitcode.Fail, err)
	}
	if err := f.Close(); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	return nil
}

// checkTrackID fails unless id has a start row, and (for end) no end row yet.
func checkTrackID(logPath, id string, forEnd bool) error {
	f, err := os.Open(logPath) //nolint:gosec // G304 -- computed from the resolved workspace
	if errors.Is(err, os.ErrNotExist) {
		return badTrack("id %s was never started", id)
	}
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	defer f.Close() //nolint:errcheck // read-only
	started, ended := false, false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		p := strings.Split(sc.Text(), trackSep)
		if len(p) < 3 || p[2] != id {
			continue
		}
		switch p[1] {
		case "start":
			started = true
		case "end":
			ended = true
		}
	}
	if err := sc.Err(); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	if !started {
		return badTrack("id %s was never started", id)
	}
	if forEnd && ended {
		return badTrack("id %s already ended", id)
	}
	return nil
}

func validTrackID(id string) error {
	if !trackIDRe.MatchString(id) {
		return badTrack("invalid track id %q", id)
	}
	return nil
}

func validTrackText(name, v string) error {
	if v == "" || strings.ContainsAny(v, "\r\n") || strings.Contains(v, trackSep) {
		return badTrack("invalid %s %q", name, v)
	}
	return nil
}

func oneOf(name, v string, allowed ...string) error {
	if !slices.Contains(allowed, v) {
		return badTrack("--%s must be one of %s, got %q", name, strings.Join(allowed, "|"), v)
	}
	return nil
}

func newTrackID(kind string, now time.Time) (string, error) {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return kind + "-" + now.Format("060102") + "-" + hex.EncodeToString(b[:]), nil
}

// unusedTrackID draws ids until one has no start row in the log (FR-7.2: ids never repeat).
func unusedTrackID(logPath, kind string, now time.Time) (string, error) {
	for range 16 {
		id, err := newTrackID(kind, now)
		if err != nil {
			return "", err
		}
		used, err := trackIDStarted(logPath, id)
		if err != nil {
			return "", err
		}
		if !used {
			return id, nil
		}
	}
	return "", fmt.Errorf("no unused %s id left for %s after 16 draws", kind, now.Format("060102"))
}

// trackIDStarted reports whether log.md already has a start row for id.
func trackIDStarted(logPath, id string) (bool, error) {
	b, err := os.ReadFile(logPath) //nolint:gosec // G304 -- computed from the resolved workspace
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(string(b), trackSep+"start"+trackSep+id+trackSep), nil
}

func newTrackStartCmd(ws *string) *cobra.Command {
	var kind, repo, est, from string
	c := &cobra.Command{
		Use:   "start --kind cook|fix|hotfix --repo <repo> --est S|M|L|- [--from <bid|issue url>]",
		Short: "Mở một lượt theo dõi và in id của nó", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			wsDir, on, err := trackWorkspace(*ws)
			if err != nil {
				return err
			}
			if !on {
				_, err = fmt.Fprintln(w, "track: off")
				return err
			}
			if err := oneOf("kind", kind, trackKinds...); err != nil {
				return err
			}
			if err := oneOf("est", est, "S", "M", "L", "-"); err != nil {
				return err
			}
			if err := validTrackText("--repo", repo); err != nil {
				return err
			}
			fields := []string{"", kind, repo, est}
			if from != "" {
				if err := checkStartFrom(trackLogPath(wsDir), from); err != nil {
					return err
				}
				fields = append(fields, from)
			}
			id, err := unusedTrackID(trackLogPath(wsDir), kind, time.Now().UTC())
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			fields[0] = id
			if err := appendTrackRow(trackLogPath(wsDir), "start", fields); err != nil {
				return err
			}
			_, err = fmt.Fprintf(w, "track: %s\n", id)
			return err
		},
	}
	c.Flags().StringVar(&kind, "kind", "", "cook|fix|hotfix")
	c.Flags().StringVar(&repo, "repo", "", "target repo name")
	c.Flags().StringVar(&est, "est", "-", "size estimate S|M|L, or - when not known yet")
	c.Flags().StringVar(&from, "from", "", "backlog id or issue URL this run takes from the backlog")
	return c
}

func newTrackEstCmd(ws *string) *cobra.Command {
	return &cobra.Command{
		Use:   "est <id> S|M|L",
		Short: "Đặt size ước lượng cho lượt đã start (dòng est sau cùng thắng)", //znf:allow-lang
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrackWrite(cmd.OutOrStdout(), *ws, 2, args, func(a []string) (string, []string, bool, error) {
				if err := validTrackID(a[0]); err != nil {
					return "", nil, false, err
				}
				if err := oneOf("size", a[1], "S", "M", "L"); err != nil {
					return "", nil, false, err
				}
				return "est", []string{a[0], a[1]}, true, nil
			})
		},
	}
}

func newTrackShipCmd(ws *string) *cobra.Command {
	var outcome, step, findings string
	var rounds int
	c := &cobra.Command{
		Use:   "ship <id> --outcome shipped|blocked [--blocked-step <step>] --findings C,H,M,L|- --rounds N",
		Short: "Ghi kết quả một lần chạy znf:ship", //znf:allow-lang
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrackWrite(cmd.OutOrStdout(), *ws, 1, args, func(a []string) (string, []string, bool, error) {
				if err := validTrackID(a[0]); err != nil {
					return "", nil, false, err
				}
				if err := oneOf("outcome", outcome, "shipped", "blocked"); err != nil {
					return "", nil, false, err
				}
				switch {
				case outcome == "blocked":
					if step == "" {
						return "", nil, false, badTrack("--blocked-step is required when --outcome blocked")
					}
					if err := oneOf("blocked-step", step, trackSteps...); err != nil {
						return "", nil, false, err
					}
				case step != "":
					return "", nil, false, badTrack("--blocked-step is only valid with --outcome blocked")
				default:
					step = "-"
				}
				if findings != "-" && !trackFindings.MatchString(findings) {
					return "", nil, false, badTrack("--findings must be - or four non-negative integers C,H,M,L, got %q", findings)
				}
				if rounds < 0 {
					return "", nil, false, badTrack("--rounds must be >= 0")
				}
				return "ship", []string{a[0], outcome, step, findings, strconv.Itoa(rounds)}, true, nil
			})
		},
	}
	c.Flags().StringVar(&outcome, "outcome", "", "shipped|blocked")
	c.Flags().StringVar(&step, "blocked-step", "", "lint|build|contract|behaviour|review (required when blocked)")
	c.Flags().StringVar(&findings, "findings", "-", "C,H,M,L finding counts, or -")
	c.Flags().IntVar(&rounds, "rounds", 0, "fix rounds the review took")
	return c
}

func newTrackFixCmd(ws *string) *cobra.Command {
	var path, regression string
	var round int
	c := &cobra.Command{
		Use:   "fix <id> --round N --path narrow|wide --regression test|no-seam|-",
		Short: "Ghi một vòng fix của lượt fix/hotfix", //znf:allow-lang
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrackWrite(cmd.OutOrStdout(), *ws, 1, args, func(a []string) (string, []string, bool, error) {
				if err := validTrackID(a[0]); err != nil {
					return "", nil, false, err
				}
				if round < 1 {
					return "", nil, false, badTrack("--round must be >= 1")
				}
				if err := oneOf("path", path, "narrow", "wide"); err != nil {
					return "", nil, false, err
				}
				if err := oneOf("regression", regression, "test", "no-seam", "-"); err != nil {
					return "", nil, false, err
				}
				return "fix", []string{a[0], strconv.Itoa(round), path, regression}, true, nil
			})
		},
	}
	c.Flags().IntVar(&round, "round", 0, "fix round number, from 1")
	c.Flags().StringVar(&path, "path", "", "narrow|wide")
	c.Flags().StringVar(&regression, "regression", "-", "test|no-seam|-")
	return c
}

func newTrackCICmd(ws *string) *cobra.Command {
	var result string
	c := &cobra.Command{
		Use:   "ci <id> --result green|red",
		Short: "Ghi kết quả CI của commit đang được watch", //znf:allow-lang
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrackWrite(cmd.OutOrStdout(), *ws, 1, args, func(a []string) (string, []string, bool, error) {
				if err := validTrackID(a[0]); err != nil {
					return "", nil, false, err
				}
				if err := oneOf("result", result, "green", "red"); err != nil {
					return "", nil, false, err
				}
				return "ci", []string{a[0], result}, true, nil
			})
		},
	}
	c.Flags().StringVar(&result, "result", "", "green|red")
	return c
}

func newTrackEndCmd(ws *string) *cobra.Command {
	var prs []string
	var branch, ledger string
	var rollback, cancelled bool
	var rounds int
	c := &cobra.Command{
		Use:   "end <id> (--pr <url>... | --branch <name> | --rollback | --cancelled) [--ledger <path>] --rounds N",
		Short: "Đóng một lượt theo dõi", //znf:allow-lang
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrackWrite(cmd.OutOrStdout(), *ws, 1, args, func(a []string) (string, []string, bool, error) {
				if err := validTrackID(a[0]); err != nil {
					return "", nil, false, err
				}
				chosen := 0
				for _, set := range []bool{len(prs) > 0, branch != "", rollback, cancelled} {
					if set {
						chosen++
					}
				}
				if chosen != 1 {
					return "", nil, false, badTrack("pass exactly one of --pr, --branch, --rollback, --cancelled")
				}
				var outcome string
				switch {
				case len(prs) > 0:
					for _, p := range prs {
						if !strings.HasPrefix(p, "http") || strings.ContainsAny(p, ", \r\n") {
							return "", nil, false, badTrack("--pr must be a URL starting with http, got %q", p)
						}
					}
					outcome = strings.Join(prs, ",")
				case branch != "":
					if err := validTrackText("--branch", branch); err != nil {
						return "", nil, false, err
					}
					outcome = "branch:" + branch
				case rollback:
					outcome = "rollback"
				default:
					outcome = "cancelled"
				}
				if rounds < 0 {
					return "", nil, false, badTrack("--rounds must be >= 0")
				}
				tasks, plan := 0, ""
				if ledger != "" {
					n, p, err := readLedger(ledger)
					tasks, plan = n, p
					switch {
					case err != nil:
						fmt.Fprintf(cmd.ErrOrStderr(), "track: read --ledger %s: %v; end row written without tasks or plan\n", ledger, err)
						tasks, plan = 0, ""
					case plan == "":
						fmt.Fprintf(cmd.ErrOrStderr(), "track: ledger %s has no \"# SDD ledger — plan:\" first line; end row written without the plan\n", ledger)
					case strings.Contains(plan, trackSep) || strings.ContainsAny(plan, "\r\n"):
						fmt.Fprintf(cmd.ErrOrStderr(), "track: plan path %q holds the field separator; end row written without the plan\n", plan)
						plan = ""
					case !filepath.IsAbs(plan):
						// Stats resolve a relative plan against the workspace, not the cwd it was written from.
						abs, aerr := filepath.Abs(plan)
						if aerr != nil {
							fmt.Fprintf(cmd.ErrOrStderr(), "track: plan path %q: %v; end row written without the plan\n", plan, aerr)
							plan = ""
						} else {
							plan = abs
						}
					}
				}
				// "end" is the row kind; checkTrackID also rejects a second end.
				fields := []string{a[0], outcome, strconv.Itoa(tasks), strconv.Itoa(rounds)}
				if plan != "" {
					fields = append(fields, plan)
				}
				return "end", fields, true, nil
			})
		},
	}
	c.Flags().StringArrayVar(&prs, "pr", nil, "pull request URL (repeatable)")
	c.Flags().StringVar(&branch, "branch", "", "pushed branch name (hotfix; the human opens the PR)")
	c.Flags().BoolVar(&rollback, "rollback", false, "the run ended in a rollback")
	c.Flags().BoolVar(&cancelled, "cancelled", false, "the run was cancelled")
	c.Flags().StringVar(&ledger, "ledger", "", "SDD ledger file; its `Task <N>: complete` lines are counted and its first-line plan path is kept")
	c.Flags().IntVar(&rounds, "rounds", 0, "fix rounds of the review in znf:ship")
	return c
}

const ledgerPlanPrefix = "# SDD ledger — plan: "

// readLedger counts the completed tasks and returns the plan path from the
// first line; plan is "" when that line does not have the SDD ledger shape.
func readLedger(path string) (tasks int, plan string, err error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- a CLI flag naming a local ledger
	if err != nil {
		return 0, "", err
	}
	for i, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if i == 0 && strings.HasPrefix(l, ledgerPlanPrefix) {
			plan = strings.TrimSpace(strings.TrimPrefix(l, ledgerPlanPrefix))
		}
		if trackTasksRe.MatchString(l) {
			tasks++
		}
	}
	return tasks, plan, nil
}

func newTrackStatsCmd(ws *string) *cobra.Command {
	var (
		dir    string
		asJSON bool
		force  bool
		today  string
	)
	c := &cobra.Command{
		Use:   "stats",
		Short: "In số liệu track đọc từ log.md và các session log", //znf:allow-lang
		Long: "Đọc <dir>/log.md và <dir>/sessions/*.log (mặc định: <workspace>/.zenify/track) rồi in\n" + //znf:allow-lang
			"số liệu hôm nay, độ chính xác estimate và chất lượng. Feature tắt thì in enabled=false\n" + //znf:allow-lang
			"và không đọc gì.", //znf:allow-lang
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			now := time.Now()
			if today == "" {
				today = now.Format("2006-01-02")
			} else if _, err := time.Parse("2006-01-02", today); err != nil {
				return badTrack("--today must be YYYY-MM-DD: %v", err)
			}
			wsDir, on, err := trackWorkspace(*ws)
			if err != nil {
				return err
			}
			var res trackstats.Result
			if on {
				if dir == "" {
					dir = filepath.Join(wsDir, ".zenify", "track")
				}
				res, err = computeTrackStats(dir, wsDir, today, now, force)
				if err != nil {
					return exitcode.New(exitcode.Fail, err)
				}
			}
			res.Enabled = on
			if asJSON {
				out, err := json.MarshalIndent(res, "", "  ")
				if err != nil {
					return exitcode.New(exitcode.Fail, err)
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", out)
				return err
			}
			renderTrackStats(cmd.OutOrStdout(), res)
			return nil
		},
	}
	c.Flags().StringVar(&dir, "dir", "", "directory holding log.md (default: <workspace>/.zenify/track)")
	c.Flags().BoolVar(&asJSON, "json", false, "output JSON instead of a short summary")
	c.Flags().BoolVar(&force, "force", false, "ignore the 5-minute backlog cache (gh issues and SDD scan)")
	c.Flags().StringVar(&today, "today", "", "the day to report, YYYY-MM-DD (default: local today)")
	return c
}

func computeTrackStats(dir, wsDir, today string, now time.Time, force bool) (trackstats.Result, error) {
	var err error
	in := trackstats.Input{
		PlansRoot: filepath.Join(wsDir, "docs", "plans"),
		Today:     today,
		Now:       now,
		Merged:    trackMerged,
		Sessions:  map[string]string{},
		Workspace: wsDir,
		PlanTitle: func(p string) (string, error) { return readPlanTitle(wsDir, p) },
	}
	in.Live, err = readLiveFiles(dir, now)
	if err != nil {
		return trackstats.Result{}, err
	}
	in.GitHub, in.SDD = backlogSources(dir, wsDir, force, now)
	b, err := os.ReadFile(filepath.Join(dir, "log.md")) //nolint:gosec // G304 -- dir is a CLI flag of a local helper
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return trackstats.Result{}, err
	}
	in.Log = string(b)
	files, err := filepath.Glob(filepath.Join(dir, "sessions", "*.log"))
	if err != nil {
		return trackstats.Result{}, err
	}
	for _, f := range files {
		sb, err := os.ReadFile(f) //nolint:gosec // G304 -- listed from the track dir
		if err != nil {
			return trackstats.Result{}, err
		}
		in.Sessions[strings.TrimSuffix(filepath.Base(f), ".log")] = string(sb)
	}
	cachePath := filepath.Join(dir, "merged.json")
	cache := map[string]string{}
	cb, err := os.ReadFile(cachePath) //nolint:gosec // G304 -- dir is a CLI flag of a local helper
	switch {
	case err == nil:
		if err := json.Unmarshal(cb, &cache); err != nil {
			return trackstats.Result{}, fmt.Errorf("parse %s: %w", cachePath, err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return trackstats.Result{}, err
	}
	in.Cache = cache
	titlesPath := filepath.Join(dir, "pr-titles.json")
	titles := map[string]string{}
	tb, err := os.ReadFile(titlesPath) //nolint:gosec // G304 -- dir is a CLI flag of a local helper
	switch {
	case err == nil:
		if err := json.Unmarshal(tb, &titles); err != nil {
			return trackstats.Result{}, fmt.Errorf("parse %s: %w", titlesPath, err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return trackstats.Result{}, err
	}
	in.PRTitles = maps.Clone(titles)
	// gh is asked about unmerged PRs at most once per mergedRecheck: the pane runs stats on
	// almost every turn, and every unmerged PR costs one sequential gh call. The stamp holds
	// the last check's error, which stays reported until the next check.
	checkedPath := filepath.Join(dir, "merged.checked")
	lastErr, err := os.ReadFile(checkedPath) //nolint:gosec // G304 -- dir is a CLI flag of a local helper
	switch {
	case err == nil:
		if fi, err := os.Stat(checkedPath); err != nil {
			return trackstats.Result{}, err
		} else if now.Sub(fi.ModTime()) < mergedRecheck {
			in.Merged = nil
		}
	case !errors.Is(err, os.ErrNotExist):
		return trackstats.Result{}, err
	}
	// A PR title costs a gh call too: ask only in the same window the merged check is allowed.
	if in.Merged != nil {
		in.PRTitle = trackPRTitle
	}
	res, newCache := trackstats.Compute(in)
	if in.Merged == nil {
		res.Today.MergedError = string(lastErr)
	} else if err := os.MkdirAll(dir, 0o750); err != nil {
		return trackstats.Result{}, err
	} else if err := writeFileAtomic(checkedPath, []byte(res.Today.MergedError), 0o644); err != nil {
		return trackstats.Result{}, err
	}
	if len(newCache) > 0 && !reflect.DeepEqual(newCache, cache) {
		out, err := json.Marshal(newCache)
		if err != nil {
			return trackstats.Result{}, err
		}
		if err := writeFileAtomic(cachePath, out, 0o644); err != nil {
			return trackstats.Result{}, err
		}
	}
	if !reflect.DeepEqual(in.PRTitles, titles) {
		out, err := json.Marshal(in.PRTitles)
		if err != nil {
			return trackstats.Result{}, err
		}
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return trackstats.Result{}, err
		}
		if err := writeFileAtomic(titlesPath, out, 0o644); err != nil {
			return trackstats.Result{}, err
		}
	}
	return res, nil
}

// readLiveFiles lists live/*.json with the mtime of each .beat (zero when it is absent).
// A file whose beat is missing or older than LiveTTL is listed without its JSON: stats ignore it.
func readLiveFiles(dir string, now time.Time) ([]trackstats.LiveFile, error) {
	files, err := filepath.Glob(filepath.Join(dir, "live", "*.json"))
	if err != nil {
		return nil, err
	}
	var live []trackstats.LiveFile
	for _, f := range files {
		lf := trackstats.LiveFile{Session: strings.TrimSuffix(filepath.Base(f), ".json")}
		switch fi, err := os.Stat(strings.TrimSuffix(f, ".json") + ".beat"); {
		case err == nil:
			lf.BeatAt = fi.ModTime()
		case !errors.Is(err, os.ErrNotExist):
			return nil, err
		}
		if lf.BeatAt.IsZero() || now.Sub(lf.BeatAt) > trackstats.LiveTTL {
			continue
		}
		b, err := os.ReadFile(f) //nolint:gosec // G304 -- listed from the track dir
		if err != nil {
			return nil, err
		}
		lf.JSON = string(b)
		live = append(live, lf)
	}
	return live, nil
}

// readPlanTitle returns the "# " heading on the first line of a plan, resolving a relative
// path against the workspace; a missing file gives "" and no error.
func readPlanTitle(wsDir, p string) (string, error) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(wsDir, p)
	}
	f, err := os.Open(p) //nolint:gosec // G304 -- path comes from this workspace's own track log
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	// Only a regular file: a FIFO or /dev/zero would hang or bloat the stats run.
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", nil
	}
	first, err := bufio.NewReader(io.LimitReader(f, 4096)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	first = strings.TrimSuffix(first, "\n")
	if t, ok := strings.CutPrefix(first, "# "); ok {
		return strings.TrimSpace(t), nil
	}
	return "", nil
}

func renderTrackStats(w io.Writer, r trackstats.Result) {
	if !r.Enabled {
		_, _ = fmt.Fprintln(w, "track: off")
		return
	}
	t := r.Today
	started, ended := 0, 0
	for _, n := range t.Started {
		started += n
	}
	for _, n := range t.Ended {
		ended += n
	}
	_, _ = fmt.Fprintf(w, "scale (%s, %d items): S<%dm M<%dm L<%dm\n", r.Scale.Source, r.Scale.N, r.Scale.S, r.Scale.M, r.Scale.L)
	_, _ = fmt.Fprintf(w, "today %s: %d started, %d ended, %d tasks, %d PRs opened, %d merged\n", t.Date, started, ended, t.Tasks, t.PRsOpened, t.Merged)
	_, _ = fmt.Fprintf(w, "running: %d\n", len(r.Running))
	if t.MergedError != "" {
		_, _ = fmt.Fprintf(w, "merged check failed: %s\n", t.MergedError)
	}
	for _, rule := range r.Rules {
		_, _ = fmt.Fprintf(w, "rule: %s\n", rule)
	}
	if r.Malformed > 0 {
		_, _ = fmt.Fprintf(w, "malformed lines: %d\n", r.Malformed)
	}
}
