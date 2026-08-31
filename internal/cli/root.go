package cli

import "github.com/spf13/cobra"

// NewRootCmd builds the top-level `zenify` command. Subcommands attach here.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "zenify",
		Short:         "zenify — portable workspace toolkit",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	return root
}
