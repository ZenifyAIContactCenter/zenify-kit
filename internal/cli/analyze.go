package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/analyze"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/spf13/cobra"
)

// runAnalyze is the testable core. FAIL-OPEN: it always returns nil; any read
// error becomes a printed note, never a process failure — this command must
// never be the reason a flow stops.
func runAnalyze(specPath, planPath string, asJSON bool, readFile func(string) ([]byte, error), stdout, stderr io.Writer) error {
	read := func(p string) string {
		if p == "" {
			return ""
		}
		b, err := readFile(p)
		if err != nil {
			fmt.Fprintf(stderr, "analyze: không phân tích được %q: %v (fail-open)\n", p, err) //znf:allow-lang
			return ""
		}
		return string(b)
	}
	specText := read(specPath)
	planText := read(planPath)
	if specText == "" && planText == "" {
		fmt.Fprintln(stderr, "analyze: không phân tích được: không đọc được spec lẫn plan (fail-open)") //znf:allow-lang
		return nil
	}
	res := analyze.Analyze(specText, planText)

	if asJSON {
		b, err := json.Marshal(res)
		if err != nil {
			fmt.Fprintln(stderr, "analyze: không phân tích được: marshal lỗi (fail-open)") //znf:allow-lang
			return nil
		}
		fmt.Fprintln(stdout, string(b))
		return nil
	}

	// human-readable
	u := ui.New(stdout)
	u.Section("znf:analyze")
	brief := "absent"
	if res.BriefFound {
		brief = fmt.Sprintf("found, %d/8 numbered fields", res.BriefFields)
	}
	u.KV([][2]string{
		{"Brief", brief},
		{"Coverage", fmt.Sprintf("%d FR in spec, %d referenced by plan", len(res.SpecFRs), len(res.PlanRefs))},
	})
	u.Blank()
	if len(res.Findings) == 0 {
		u.Note("No mechanical findings.")
	} else {
		u.Step(ui.StatusWarn, fmt.Sprintf("%d finding(s)", len(res.Findings)),
			fmt.Sprintf("CRITICAL=%d HIGH=%d", res.SeverityCounts["CRITICAL"], res.SeverityCounts["HIGH"]))
		for _, f := range res.Findings {
			id := f.ID
			if id == "" {
				id = f.Location
			}
			u.Note(fmt.Sprintf("[%s] %s %s — %s", f.Severity, f.Kind, id, f.Message))
		}
	}
	u.Blank()
	u.Note("Advisory — mechanical scan only; does not block. Judgment passes run in znf:analyze.")
	return nil
}

func newAnalyzeCmd() *cobra.Command {
	var specPath, planPath string
	var asJSON bool
	c := &cobra.Command{
		Use:   "analyze",
		Short: "Phân tích cơ học cặp spec+plan (coverage, marker, cấu trúc Brief) — advisory, fail-open", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAnalyze(specPath, planPath, asJSON, os.ReadFile, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	c.Flags().StringVar(&specPath, "spec", "", "path to the spec markdown file")
	c.Flags().StringVar(&planPath, "plan", "", "path to the plan markdown file")
	c.Flags().BoolVar(&asJSON, "json", false, "emit the machine-readable Result JSON")
	return c
}
