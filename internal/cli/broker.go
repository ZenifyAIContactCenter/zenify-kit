package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	kit "github.com/ZenifyAIContactCenter/zenify-kit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/brokerclient"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/spf13/cobra"
)

// newBrokerCmd builds `zenify broker token|headers`.
func newBrokerCmd() *cobra.Command {
	var manifestPath, overlayPath string
	var refresh bool
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
			var tok, login, role string
			if refresh {
				// Best effort: the Claude Code Bash sandbox denies writes under ~/.zenify.
				if derr := c.DropCache(); derr != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: broker token cache not removed: %v\n", derr)
				}
				tok, login, role, err = c.Refresh(cmd.Context())
			} else {
				tok, login, role, err = c.TokenInfo(cmd.Context())
			}
			if err != nil {
				return err
			}
			// stderr, so `$(zenify broker token)` still captures only the JWT.
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "login=%s, role=%s\n", login, role)
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
			if c.HTTP == nil {
				c.HTTP = brokerclient.NewHTTPClient(brokerHeadersTimeout)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), brokerHeadersTimeout)
			defer cancel()
			tok, err := c.Token(ctx)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"Authorization": "Bearer " + tok})
		},
	}
	for _, c := range []*cobra.Command{token, headers} {
		c.Flags().StringVar(&manifestPath, "manifest", "", brokerManifestUsage)
		c.Flags().StringVar(&overlayPath, "overlay", "", brokerOverlayUsage)
	}
	token.Flags().BoolVar(&refresh, "refresh", false, "drop the cached JWT and fetch a new one (use when the broker refuses the cached token)")
	cmd.AddCommand(token, headers)
	return cmd
}

// brokerHeadersTimeout bounds `broker headers` (gh and HTTP together): Claude
// Code drops a headersHelper after 10s.
var brokerHeadersTimeout = 8 * time.Second

// ghToken supplies the GitHub token for every broker client the cli builds;
// internal/cli's TestMain swaps it for a stub so no test can run a real gh.
var ghToken = brokerclient.GHTokenExec

// ghRunner is the `gh` runner every command and doctor check uses; TestMain
// swaps it for one that panics, so no test can run a real gh.
var ghRunner ghx.Runner = ghx.ExecRunner()

// currentGH defers to ghRunner at call time, so a check registered once still
// sees a runner swapped later.
type currentGH struct{}

func (currentGH) Run(args ...string) ([]byte, error) { return ghRunner.Run(args...) }

// Flag help shared by every command that resolves the broker.
const (
	brokerManifestUsage = "path to the repos manifest whose broker: decides the route (default: the manifest embedded in the binary, never cwd)"
	brokerOverlayUsage  = "path to a personal overlay (default: none; an overlay never changes broker:)"
)

// trustedBrokerManifest is the manifest the broker comes from when no
// --manifest is given: the copy embedded in the binary, never a cwd file, since
// the broker URL receives the developer's GitHub token. internal/cli's
// TestMain swaps it for a manifest without a broker.
var trustedBrokerManifest = func() ([]byte, string) { return kit.DefaultRepos, "embedded" }

// brokerManifest is the one resolver for broker config: an explicit path (a
// missing or invalid file is an error, never a fallback to local credentials)
// or trustedBrokerManifest. The overlay only rewrites repo paths.
func brokerManifest(explicit, overlayPath string) (*manifest.Manifest, error) {
	if explicit != "" {
		return manifest.LoadWithOverlay(explicit, overlayPath)
	}
	b, label := trustedBrokerManifest()
	return manifest.ParseWithOverlay(b, label, overlayPath)
}

// newBrokerClient is the one constructor for a manifest's broker client, so the
// GitHub-token and tuneBrokerClient seams cannot be skipped.
func newBrokerClient(m *manifest.Manifest) (*brokerclient.Client, bool) {
	c, ok := brokerclient.FromManifest(m)
	if ok {
		c.GHToken = ghToken
		if tuneBrokerClient != nil {
			tuneBrokerClient(c)
		}
	}
	return c, ok
}

// tuneBrokerClient lets a test point the client at an httptest server.
var tuneBrokerClient func(*brokerclient.Client)

// brokerFromFlags resolves the broker via brokerManifest and returns its
// client; ok is false when that manifest declares none.
func brokerFromFlags(manifestPath, overlayPath string) (c *brokerclient.Client, ok bool, err error) {
	m, err := brokerManifest(manifestPath, overlayPath)
	if err != nil {
		return nil, false, err
	}
	c, ok = newBrokerClient(m)
	return c, ok, nil
}

// brokerDeclared reports whether the trusted manifest declares a broker.
func brokerDeclared() bool {
	m, err := brokerManifest("", "")
	return err == nil && m.Broker != nil
}
