package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/standards"
	"github.com/spf13/cobra"
)

// runStandards is the testable core. FAIL-OPEN: always returns nil; any read
// error becomes a printed note, never a process failure.
func runStandards(specPath, planPath, root string, asJSON bool, readFile func(string) ([]byte, error), stdout, stderr io.Writer) error {
	read := func(p string) string {
		if p == "" {
			return ""
		}
		b, err := readFile(p)
		if err != nil {
			fmt.Fprintf(stderr, "standards: không đọc được %q: %v (fail-open)\n", p, err)
			return ""
		}
		return string(b)
	}
	specText := read(specPath)
	planText := read(planPath)
	if specText == "" || planText == "" {
		fmt.Fprintln(stderr, "standards: cần cả spec lẫn plan để đối chiếu FR↔test (fail-open)")
		return nil
	}
	if root == "" {
		root = "."
	}
	res := standards.Check(specText, planText, root, readFile)

	if asJSON {
		b, err := json.Marshal(res)
		if err != nil {
			fmt.Fprintln(stderr, "standards: marshal lỗi (fail-open)")
			return nil
		}
		fmt.Fprintln(stdout, string(b))
		return nil
	}

	fmt.Fprintf(stdout, "Test-traceability: %d declared test path(s)\n", len(res.TestPaths))
	if len(res.Findings) == 0 {
		fmt.Fprintln(stdout, "No mechanical findings — every FR has a declared test on disk.")
	} else {
		fmt.Fprintf(stdout, "%d finding(s) [HIGH=%d MEDIUM=%d INFO=%d]:\n",
			len(res.Findings), res.SeverityCounts["HIGH"], res.SeverityCounts["MEDIUM"], res.SeverityCounts["INFO"])
		for _, f := range res.Findings {
			id := f.ID
			if id == "" {
				id = f.Location
			}
			fmt.Fprintf(stdout, "  [%s] %s %s — %s\n", f.Severity, f.Kind, id, f.Message)
		}
	}
	fmt.Fprintln(stdout, "\n(Advisory — mechanical scan only; does not block. Judgment passes run in znf:standards.)")
	return nil
}

func newStandardsCmd() *cobra.Command {
	var specPath, planPath, root string
	var asJSON bool
	c := &cobra.Command{
		Use:   "standards",
		Short: "Check test-traceability — every FR has a real test on disk (advisory, fail-open)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStandards(specPath, planPath, root, asJSON, os.ReadFile, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	c.Flags().StringVar(&specPath, "spec", "", "path to the spec markdown file")
	c.Flags().StringVar(&planPath, "plan", "", "path to the plan markdown file")
	c.Flags().StringVar(&root, "root", "", "directory test paths resolve against (default: current dir)")
	c.Flags().BoolVar(&asJSON, "json", false, "emit the machine-readable Result JSON")
	return c
}
