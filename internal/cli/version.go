package cli

import (
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "In version của zenify", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(version.Current())
			return nil
		},
	}
}
