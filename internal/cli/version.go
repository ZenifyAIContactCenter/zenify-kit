package cli

import (
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the zenify version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(version.Current())
			return nil
		},
	}
}
