package cli

import (
	"encoding/json"
	"errors"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
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

const doctorSchemaVersion = 1

type doctorCheckJSON struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}
type doctorData struct {
	Healthy bool              `json:"healthy"`
	Checks  []doctorCheckJSON `json:"checks"`
}
type doctorEnvelope struct {
	SchemaVersion int        `json:"schema_version"`
	Data          doctorData `json:"data"`
}

func newDoctorCmd() *cobra.Command {
	var asJSON, exitOnFail, doFix bool
	cmd := &cobra.Command{
		Use:           "doctor",
		Short:         "Read-only environment health check (never mutates, never prints secrets)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if doFix {
				results, _ := runChecks()
				for i, r := range results {
					if !r.OK && checks[i].Fix != nil {
						ok, detail := checks[i].Fix()
						mark := "⚙"
						if !ok {
							mark = "✗"
						}
						if !asJSON {
							cmd.Printf("%s %s: %s\n", mark, checks[i].Name, detail)
						}
					}
				}
			}
			results, healthy := runChecks()
			if asJSON {
				env := doctorEnvelope{SchemaVersion: doctorSchemaVersion, Data: doctorData{Healthy: healthy}}
				for _, r := range results {
					env.Data.Checks = append(env.Data.Checks, doctorCheckJSON(r))
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(env); err != nil {
					return err
				}
			} else {
				for _, r := range results {
					mark := "✓"
					if !r.OK {
						mark = "✗"
					}
					cmd.Printf("%s %s: %s\n", mark, r.Name, r.Detail)
				}
			}
			if exitOnFail && !healthy {
				return exitcode.New(exitcode.Fail, errors.New("doctor: one or more checks failed"))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit a machine-readable JSON envelope")
	cmd.Flags().BoolVar(&exitOnFail, "exit-on-fail", false, "exit non-zero if any check fails")
	cmd.Flags().BoolVar(&doFix, "fix", false, "apply the safe-subset of automatic repairs, then re-check")
	return cmd
}
