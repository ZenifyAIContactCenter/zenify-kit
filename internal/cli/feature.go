package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func newFeatureCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "feature [<key> on|off]",
		Short: "Liệt kê hoặc bật/tắt tính năng opt-in của kit trong overlay workspace", //znf:allow-lang
		Args:  cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if workspace == "" {
				workspace = workspaceOrCwd("", cmd.ErrOrStderr())
			}
			overlay := filepath.Join(workspace, ".zenify-overlay.yaml")
			out := cmd.OutOrStdout()
			switch len(args) {
			case 0:
				feats, err := manifest.ReadFeatures(overlay)
				if err != nil {
					return err
				}
				for _, k := range manifest.KnownFeatures {
					fmt.Fprintf(out, "%s %s\n", k, onOff(feats[k]))
				}
				return nil
			case 1:
				return fmt.Errorf("usage: zenify feature <key> on|off")
			}
			if args[1] != "on" && args[1] != "off" {
				return fmt.Errorf("value must be on or off, got %q", args[1])
			}
			if err := manifest.SetFeature(overlay, args[0], args[1] == "on"); err != nil {
				return err
			}
			fmt.Fprintf(out, "%s %s\n", args[0], args[1])
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace root (default: resolved workspace or cwd)")
	return cmd
}
