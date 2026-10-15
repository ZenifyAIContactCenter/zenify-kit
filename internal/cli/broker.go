package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/brokerclient"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/spf13/cobra"
)

// newBrokerCmd builds `zenify broker token`.
func newBrokerCmd() *cobra.Command {
	var manifestPath, overlayPath string
	cmd := &cobra.Command{
		Use:   "broker",
		Short: "Làm việc với credential broker của zenify", //znf:allow-lang
	}
	token := &cobra.Command{
		Use:           "token",
		Short:         "In JWT của broker ra stdout (dùng cache, tự xin lại khi còn dưới 60 giây)", //znf:allow-lang
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if overlayPath == "" {
				cwd, _ := os.Getwd()
				if ws, _, ok := resolveWorkspace(cwd, "", os.Getenv, os.UserHomeDir); ok {
					overlayPath = filepath.Join(ws, ".zenify-overlay.yaml")
				}
			}
			m, _, err := loadKitManifest(manifestPath, overlayPath)
			if err != nil {
				return err
			}
			c, ok := brokerclient.FromManifest(m)
			if !ok {
				return exitcode.New(exitcode.BadArgs, errors.New("manifest không khai báo broker")) //znf:allow-lang
			}
			tok, err := c.Token(cmd.Context())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), tok)
			return err
		},
	}
	token.Flags().StringVar(&manifestPath, "manifest", "", "path to the repos manifest (default: manifest/repos.yaml under cwd, else the embedded one)")
	token.Flags().StringVar(&overlayPath, "overlay", "", "path to personal overlay (default <workspace>/.zenify-overlay.yaml)")
	cmd.AddCommand(token)
	return cmd
}
