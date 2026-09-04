package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/analyze"
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
			fmt.Fprintf(stderr, "analyze: không phân tích được %q: %v (fail-open)\n", p, err)
			return ""
		}
		return string(b)
	}
	specText := read(specPath)
	planText := read(planPath)
	if specText == "" && planText == "" {
		fmt.Fprintln(stderr, "analyze: không phân tích được: không đọc được spec lẫn plan (fail-open)")
		return nil
	}
	res := analyze.Analyze(specText, planText)

	if asJSON {
		b, err := json.Marshal(res)
		if err != nil {
			fmt.Fprintln(stderr, "analyze: không phân tích được: marshal lỗi (fail-open)")
			return nil
		}
		fmt.Fprintln(stdout, string(b))
		return nil
	}

	// human-readable
	fmt.Fprintf(stdout, "Brief: ")
	if res.BriefFound {
		fmt.Fprintf(stdout, "found, %d/7 numbered fields\n", res.BriefFields)
	} else {
		fmt.Fprintf(stdout, "absent\n")
	}
	fmt.Fprintf(stdout, "Coverage: %d FR in spec, %d referenced by plan\n", len(res.SpecFRs), len(res.PlanRefs))
	if len(res.Findings) == 0 {
		fmt.Fprintln(stdout, "No mechanical findings.")
	} else {
		fmt.Fprintf(stdout, "%d finding(s) [CRITICAL=%d HIGH=%d]:\n",
			len(res.Findings), res.SeverityCounts["CRITICAL"], res.SeverityCounts["HIGH"])
		for _, f := range res.Findings {
			id := f.ID
			if id == "" {
				id = f.Location
			}
			fmt.Fprintf(stdout, "  [%s] %s %s — %s\n", f.Severity, f.Kind, id, f.Message)
		}
	}
	fmt.Fprintln(stdout, "\n(Advisory — mechanical scan only; does not block. Judgment passes run in znf:analyze.)")
	return nil
}

func newAnalyzeCmd() *cobra.Command {
	var specPath, planPath string
	var asJSON bool
	c := &cobra.Command{
		Use:   "analyze",
		Short: "Mechanically analyze a spec+plan pair (coverage, markers, Brief structure) — advisory, fail-open",
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
