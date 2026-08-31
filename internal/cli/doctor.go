package cli

import (
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
	"github.com/spf13/cobra"
)

// Check is one read-only diagnostic. B/C/D register more via RegisterCheck.
type Check struct {
	Name string
	Run  func() (ok bool, detail string)
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
