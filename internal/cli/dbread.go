package cli

import (
	"os"
	"os/exec"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/dbread"
	"github.com/spf13/cobra"
)

// newDBReadCmd builds `zenify db-read <collections|tables|doc|count|eval|sql> [arg]`
// — the portable port of ~/.local/bin/db_read (FR-033).
func newDBReadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "db-read <collections|tables|doc|count|eval|sql> [arg]",
		Short:         "Read-only inspection of the zenify shared databases",
		Args:          cobra.RangeArgs(1, 2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := ""
			if len(args) > 1 {
				arg = args[1]
			}
			o := &dbread.Options{
				Cmd:    args[0],
				Arg:    arg,
				Stdout: cmd.OutOrStdout(),
				Stderr: cmd.ErrOrStderr(),
				Env:    os.Getenv,
			}
			o.SetRun(func(name string, args []string, extraEnv []string, _ string) error {
				c := exec.Command(name, args...) //nolint:gosec // G204 -- name is always "mongosh" or "mysql" from this package's own callers, args are internally-computed subcommands, not attacker-controlled shell input
				c.Env = append(os.Environ(), extraEnv...)
				c.Stdout = o.Stdout
				c.Stderr = o.Stderr
				return c.Run()
			})
			return dbread.Run(o)
		},
	}
	return cmd
}
