package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/review"
	"github.com/spf13/cobra"
)

// runReviewVerify là core testable: đọc mảng findings JSON từ stdin, verify cơ học,
// in Result JSON ra stdout. FAIL-OPEN: input rỗng/hỏng → không mất finding, không lỗi.
func runReviewVerify(stdin io.Reader, stdout, stderr io.Writer, readFile func(string) ([]byte, error)) error {
	data, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "review-verify: đọc stdin lỗi — fail-open (giữ nguyên)")
		return nil
	}
	if len(bytes.TrimSpace(data)) == 0 {
		fmt.Fprintln(stdout, `{"findings":[],"kept":0,"refuted":0}`)
		return nil
	}
	var findings []review.Finding
	if err := json.Unmarshal(data, &findings); err != nil {
		fmt.Fprintln(stderr, "review-verify: stdin không phải JSON findings — fail-open (giữ nguyên)")
		_, _ = stdout.Write(data)
		return nil
	}
	res := review.Verify(findings, readFile)
	out, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintln(stderr, "review-verify: marshal lỗi — fail-open (giữ nguyên)")
		_, _ = stdout.Write(data)
		return nil
	}
	fmt.Fprintln(stdout, string(out))
	return nil
}

// runReviewBundle là core testable: chạy rundiff(base) lấy `git diff --numstat`,
// parse thành FileStat, chia bundle, in Plan JSON ra stdout.
// FAIL-OPEN: diff lỗi/marshal lỗi → in passthrough, KHÔNG trả error (engine rơi về đường cũ).
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
		return passthrough("review-bundle: git diff lỗi — fail-open (passthrough)")
	}
	plan := review.PlanBundles(parseNumstat(out), 2000, 600, 8)
	b, err := json.Marshal(plan)
	if err != nil {
		return passthrough("review-bundle: marshal lỗi — fail-open (passthrough)")
	}
	fmt.Fprintln(stdout, string(b))
	return nil
}

// parseNumstat đọc output `git diff --numstat`: mỗi dòng "<added>\t<deleted>\t<path>".
// Dòng binary là "-\t-\t<path>" → LOC 0. Bỏ dòng rỗng/không đủ cột.
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

// runReviewDoctrine: stdin = text (thường là block ## Verified) → JSON đã sanitize.
// Fail-open: đọc lỗi → emit rỗng, exit 0.
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
		fmt.Fprintln(stderr, "review-doctrine: lỗi đọc stdin, fail-open:", err)
		return emit("", nil)
	}
	clean, stripped := review.SanitizeVerified(string(data))
	return emit(clean, stripped)
}

func newReviewDoctrineCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "review-doctrine",
		Short:  "Cơ học strip dòng chỉ-verdict khỏi ## Verified của ship-pack (text qua stdin, seam doctrine)",
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
// Cơ học quyết định có gọi adviser LLM không ở POST. Fail-open: đọc/parse lỗi
// hoặc rỗng → {"advise":false,"signals":[]}, exit 0.
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
		fmt.Fprintln(stderr, "review-advise-gate: lỗi đọc stdin, fail-open:", err)
		return emit(false, nil)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return emit(false, nil)
	}
	var in review.AdviseInput
	if err := json.Unmarshal(data, &in); err != nil {
		fmt.Fprintln(stderr, "review-advise-gate: stdin không phải JSON AdviseInput, fail-open:", err)
		return emit(false, nil)
	}
	advise, signals := review.AdviseGate(in)
	return emit(advise, signals)
}

func newReviewAdviseGateCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "review-advise-gate",
		Short:  "Cơ học quyết định có gọi adviser LLM không ở POST của znf:review (AdviseInput JSON qua stdin, seam POST)",
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
		Short:  "Cơ học chia diff lớn thành bundle cụm-file cho znf:review (seam BUNDLE)",
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
				return exec.Command("git", "diff", "--no-renames", "--numstat", base).Output()
			}
			return runReviewBundle(args[0], rundiff, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func newReviewVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "review-verify",
		Short:  "Cơ học verify findings của znf:review vs file thật (findings JSON qua stdin, seam VERIFY)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReviewVerify(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), os.ReadFile)
		},
	}
}
