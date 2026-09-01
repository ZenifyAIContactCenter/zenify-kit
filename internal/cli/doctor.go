package cli

import (
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
	"github.com/spf13/cobra"
)

// Check is one read-only diagnostic. B/C/D/F register more via RegisterCheck.
// Fix is an optional mutating repair, run ONLY under `doctor --fix`; nil means
// the check reports but cannot self-repair. Run must never mutate.
type Check struct {
	Name string
	Run  func() (ok bool, detail string)
	Fix  func() (fixed bool, detail string)
}

// CheckResult is one check's outcome, ready for either renderer.
type CheckResult struct {
	Name   string
	OK     bool
	Detail string
}

// runChecks runs every registered check's Run in order and reports whether all
// passed. Pure: no output, no mutation — the renderers (human/JSON) consume it.
func runChecks() (results []CheckResult, healthy bool) {
	healthy = true
	for _, c := range checks {
		ok, detail := c.Run()
		if !ok {
			healthy = false
		}
		results = append(results, CheckResult{Name: c.Name, OK: ok, Detail: detail})
	}
	return results, healthy
}

var checks = []Check{
	{Name: "version", Run: func() (bool, string) { return true, version.Current() }},
}

// RegisterCheck is the exported seam other subsystems (B/C/D) call to add
// diagnostics without editing this file.
func RegisterCheck(c Check) { checks = append(checks, c) }

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Read-only environment health check (never mutates, never prints secrets)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, c := range checks {
				ok, detail := c.Run()
				mark := "✓"
				if !ok {
					mark = "✗"
				}
				cmd.Printf("%s %s: %s\n", mark, c.Name, detail)
			}
			return nil
		},
	}
}
