package cli

import (
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
	"github.com/spf13/cobra"
)

// NewRootCmd builds the top-level `zenify` command. Subcommands attach here.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "zenify",
		Short:         "zenify — portable workspace toolkit",
		Version:       version.Current(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// A flag-parse error is a usage error → exit 2 (BadArgs), not 1.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return exitcode.New(exitcode.BadArgs, err)
	})
	root.AddCommand(newVersionCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newUpCmd())
	root.AddCommand(newWtCmd())
	root.AddCommand(newDBReadCmd())
	return root
}
