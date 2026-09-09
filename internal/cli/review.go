package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/review"
	"github.com/spf13/cobra"
)

// runReviewVerify is the testable core: reads a JSON array of findings from stdin, verifies
// mechanically, prints the Result JSON to stdout. FAIL-OPEN: empty/corrupt input → no finding
// is lost, no error.
func runReviewVerify(stdin io.Reader, stdout, stderr io.Writer, readFile func(string) ([]byte, error)) error {
	data, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "review-verify: đọc stdin lỗi — fail-open (giữ nguyên)") //znf:allow-lang
		return nil
	}
	if len(bytes.TrimSpace(data)) == 0 {
		fmt.Fprintln(stdout, `{"findings":[],"kept":0,"refuted":0}`)
		return nil
	}
	var findings []review.Finding
	if err := json.Unmarshal(data, &findings); err != nil {
		fmt.Fprintln(stderr, "review-verify: stdin không phải JSON findings — fail-open (giữ nguyên)") //znf:allow-lang
		_, _ = stdout.Write(data)
		return nil
	}
	res := review.Verify(findings, readFile)
	out, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintln(stderr, "review-verify: marshal lỗi — fail-open (giữ nguyên)") //znf:allow-lang
		_, _ = stdout.Write(data)
		return nil
	}
	fmt.Fprintln(stdout, string(out))
	return nil
}

// runReviewBundle is the testable core: runs rundiff(base) to get `git diff --numstat`,
// parses it into FileStat, splits into bundles, prints the Plan JSON to stdout.
// FAIL-OPEN: diff error/marshal error → prints passthrough, does NOT return an error (the
// engine falls back to the old path).
func runReviewBundle(base string, rundiff func(string) ([]byte, error), stdout, stderr io.Writer) error {
	passthrough := func(note string) error {
		if note != "" {
			fmt.Fprintln(stderr, note)
		}
		fmt.Fprintln(stdout, `{"verdict":"passthrough","bundles":[],"total_loc":0}`)
		return nil
	}
	out, err := rundiff(base)
	if err != nil {
		return passthrough("review-bundle: git diff lỗi — fail-open (passthrough)") //znf:allow-lang
	}
	plan := review.PlanBundles(parseNumstat(out), 2000, 600, 8)
	b, err := json.Marshal(plan)
	if err != nil {
		return passthrough("review-bundle: marshal lỗi — fail-open (passthrough)") //znf:allow-lang
	}
	fmt.Fprintln(stdout, string(b))
	return nil
}

// parseNumstat reads the output of `git diff --numstat`: each line is "<added>\t<deleted>\t<path>".
// A binary line is "-\t-\t<path>" → LOC 0. Skip empty lines or lines with too few columns.
func parseNumstat(out []byte) []review.FileStat {
	var files []review.FileStat
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		files = append(files, review.FileStat{Path: parts[2], LOC: atoiOr0(parts[0]) + atoiOr0(parts[1])})
	}
	return files
}

func atoiOr0(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

type doctrineResult struct {
	Verified string   `json:"verified"`
	Stripped []string `json:"stripped"`
}

// runReviewDoctrine: stdin = text (usually the ## Verified block) → sanitized JSON.
// Fail-open: read error → emit empty, exit 0.
func runReviewDoctrine(stdin io.Reader, stdout, stderr io.Writer) error {
	emit := func(clean string, stripped []string) error {
		if stripped == nil {
			stripped = []string{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(doctrineResult{Verified: clean, Stripped: stripped})
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "review-doctrine: lỗi đọc stdin, fail-open:", err) //znf:allow-lang
		return emit("", nil)
	}
	clean, stripped := review.SanitizeVerified(string(data))
	return emit(clean, stripped)
}

func newReviewDoctrineCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "review-doctrine",
		Short:  "Cơ học strip dòng chỉ-verdict khỏi ## Verified của ship-pack (text qua stdin, seam doctrine)", //znf:allow-lang
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReviewDoctrine(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

type adviseResult struct {
	Advise  bool     `json:"advise"`
	Signals []string `json:"signals"`
}

// runReviewAdviseGate: stdin = AdviseInput JSON → {"advise":..,"signals":[..]}.
// Mechanically decides whether to call the adviser LLM at POST. Fail-open: read/parse error
// or empty → {"advise":false,"signals":[]}, exit 0.
func runReviewAdviseGate(stdin io.Reader, stdout, stderr io.Writer) error {
	emit := func(advise bool, signals []string) error {
		if signals == nil {
			signals = []string{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(adviseResult{Advise: advise, Signals: signals})
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "review-advise-gate: lỗi đọc stdin, fail-open:", err) //znf:allow-lang
		return emit(false, nil)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return emit(false, nil)
	}
	var in review.AdviseInput
	if err := json.Unmarshal(data, &in); err != nil {
		fmt.Fprintln(stderr, "review-advise-gate: stdin không phải JSON AdviseInput, fail-open:", err) //znf:allow-lang
		return emit(false, nil)
	}
	advise, signals := review.AdviseGate(in)
	return emit(advise, signals)
}

func newReviewAdviseGateCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "review-advise-gate",
		Short:  "Cơ học quyết định có gọi adviser LLM không ở POST của znf:review (AdviseInput JSON qua stdin, seam POST)", //znf:allow-lang
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReviewAdviseGate(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func newReviewBundleCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "review-bundle",
		Short:  "Cơ học chia diff lớn thành bundle cụm-file cho znf:review (seam BUNDLE)", //znf:allow-lang
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rundiff := func(base string) ([]byte, error) {
				// --no-renames: with rename detection, a rename emits an arrow-form path
				// ("… foo/{old => new}.go") that the per-bundle `git diff -- <files>` cannot
				// match, silently dropping a renamed-with-content file from its bundle's
				// scoped diff. --no-renames splits every rename into a delete + an add, each
				// with a real path that matches. Accepted tradeoff: a rename now counts its
				// LOC on BOTH lines (a pure rename → 2×filesize, not 0), inflating TotalLOC —
				// but only ever in the over-bundle (safe) direction; correct diff matching
				// beats exact LOC accounting.
				return exec.Command("git", "diff", "--no-renames", "--numstat", base).Output() //nolint:gosec // G204 -- fixed git subcommand; base is a validated ref, not shell input
			}
			return runReviewBundle(args[0], rundiff, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

// gitCommonDirRun runs the real `git <args>` (a seam for test injection).
func gitCommonDirRun(args ...string) ([]byte, error) {
	return exec.Command("git", args...).Output() //nolint:gosec // G204 -- fixed 'git' binary; args are internal, not user shell input
}

// reviewLogDir resolves the learning-capture store at the MAIN checkout: the parent of
// git-common-dir + /.znf/review-log. From a worktree, git-common-dir points at <main>/.git,
// so parent = main checkout → the record survives `wt rm`.
func reviewLogDir(run func(...string) ([]byte, error)) (string, error) {
	out, err := run("rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	gcd := strings.TrimSpace(string(out))
	if gcd == "" {
		return "", fmt.Errorf("empty git-common-dir")
	}
	if !filepath.IsAbs(gcd) {
		abs, err := filepath.Abs(gcd)
		if err != nil {
			return "", err
		}
		gcd = abs
	}
	return filepath.Join(filepath.Dir(gcd), ".znf", "review-log"), nil
}

func defaultReviewLogDir() (string, error) { return reviewLogDir(gitCommonDirRun) }

// runReviewLogRecord: stdin = Record JSON → writes it to the store. Best-effort, ALWAYS
// exits 0: empty/corrupt/resolve-dir error/write error → silently skipped (note to stderr),
// NO stdout, NO returned error. Capture must never block a review.
func runReviewLogRecord(stdin io.Reader, stderr io.Writer, dirFn func() (string, error)) error {
	data, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "review-log record: đọc stdin lỗi, bỏ qua:", err) //znf:allow-lang
		return nil
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	var r review.Record
	if err := json.Unmarshal(data, &r); err != nil {
		fmt.Fprintln(stderr, "review-log record: JSON hỏng, bỏ qua:", err) //znf:allow-lang
		return nil
	}
	dir, err := dirFn()
	if err != nil {
		fmt.Fprintln(stderr, "review-log record: không resolve store (không phải git repo?), bỏ qua:", err) //znf:allow-lang
		return nil
	}
	if _, err := review.WriteRecord(dir, r); err != nil {
		fmt.Fprintln(stderr, "review-log record: ghi lỗi, bỏ qua:", err) //znf:allow-lang
		return nil
	}
	return nil
}

// runReviewLogShow reads the store → a human-readable summary; --json → the full record array
// (for M6). Empty/missing/errored dir → "no reviews logged yet", exit 0. Read-only.
func runReviewLogShow(stdout, stderr io.Writer, asJSON bool, dirFn func() (string, error)) error {
	none := func() error { fmt.Fprintln(stdout, "no reviews logged yet"); return nil }
	dir, err := dirFn()
	if err != nil {
		return none()
	}
	recs, err := review.LoadRecords(dir)
	if err != nil {
		return none()
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
	s := review.Summarize(recs)
	fmt.Fprintf(stdout, "reviews: %d\n", s.Total)
	for _, tier := range []string{"T1", "T2", "T3"} {
		if n := s.ByTier[tier]; n > 0 {
			fmt.Fprintf(stdout, "  %s: %d\n", tier, n)
		}
	}
	fmt.Fprintf(stdout, "findings (kept): C%d H%d M%d L%d\n", s.Findings.Critical, s.Findings.High, s.Findings.Medium, s.Findings.Low)
	fmt.Fprintf(stdout, "kept %d / refuted %d (refute rate %.0f%%)\n", s.Kept, s.Refuted, s.RefuteRate*100)
	fmt.Fprintf(stdout, "shippable: %d/%d\n", s.ShippableN, s.Total)
	if len(s.TopCategory) > 0 {
		fmt.Fprint(stdout, "top categories:")
		for i, c := range s.TopCategory {
			if i >= 5 {
				break
			}
			fmt.Fprintf(stdout, " %s(%d)", c.Name, c.N)
		}
		fmt.Fprintln(stdout)
	}
	return nil
}

func newReviewLogCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "review-log",
		Short: "Xem learning-capture log của znf:review (summary local; --json cho M6)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReviewLogShow(cmd.OutOrStdout(), cmd.ErrOrStderr(), asJSON, defaultReviewLogDir)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "in toàn bộ record JSON (cho M6 sync)") //znf:allow-lang
	record := &cobra.Command{
		Use:    "record",
		Short:  "Ghi một review record vào store local (Record JSON qua stdin, seam POST)", //znf:allow-lang
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReviewLogRecord(cmd.InOrStdin(), cmd.ErrOrStderr(), defaultReviewLogDir)
		},
	}
	c.AddCommand(record)
	return c
}

func newReviewVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "review-verify",
		Short:  "Cơ học verify findings của znf:review vs file thật (findings JSON qua stdin, seam VERIFY)", //znf:allow-lang
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReviewVerify(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), os.ReadFile)
		},
	}
}
