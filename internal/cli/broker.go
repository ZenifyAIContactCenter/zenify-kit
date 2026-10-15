package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/brokerclient"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/spf13/cobra"
)

// newBrokerCmd builds `zenify broker token|headers`.
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
			c, ok, err := brokerFromFlags(manifestPath, overlayPath)
			if err != nil {
				return err
			}
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
	headers := &cobra.Command{
		Use:           "headers",
		Short:         "In header Authorization cho MCP http của Claude Code (JSON một object)", //znf:allow-lang
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, ok, err := brokerFromFlags(manifestPath, overlayPath)
			if err != nil {
				return err
			}
			if !ok {
				return exitcode.New(exitcode.BadArgs, errors.New("manifest không khai báo broker")) //znf:allow-lang
			}
			// Claude Code drops a headersHelper after 10s.
			if c.HTTP == nil {
				c.HTTP = &http.Client{Timeout: 8 * time.Second}
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 8*time.Second)
			defer cancel()
			tok, err := c.Token(ctx)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"Authorization": "Bearer " + tok})
		},
	}
	for _, c := range []*cobra.Command{token, headers} {
		c.Flags().StringVar(&manifestPath, "manifest", "", "path to the repos manifest (default: manifest/repos.yaml under cwd, else the embedded one)")
		c.Flags().StringVar(&overlayPath, "overlay", "", "path to personal overlay (default <workspace>/.zenify-overlay.yaml)")
	}
	cmd.AddCommand(token, headers)
	return cmd
}

// tuneBrokerClient lets a test point the client at an httptest server.
var tuneBrokerClient func(*brokerclient.Client)

// brokerFromFlags loads the kit manifest (overlay defaulting to the workspace
// one) and returns its broker client; ok is false when it declares none.
func brokerFromFlags(manifestPath, overlayPath string) (c *brokerclient.Client, ok bool, err error) {
	if overlayPath == "" {
		cwd, _ := os.Getwd()
		if ws, _, found := resolveWorkspace(cwd, "", os.Getenv, os.UserHomeDir); found {
			overlayPath = filepath.Join(ws, ".zenify-overlay.yaml")
		}
	}
	m, _, err := loadKitManifest(manifestPath, overlayPath)
	if err != nil {
		return nil, false, err
	}
	c, ok = brokerclient.FromManifest(m)
	if ok && tuneBrokerClient != nil {
		tuneBrokerClient(c)
	}
	return c, ok, nil
}
