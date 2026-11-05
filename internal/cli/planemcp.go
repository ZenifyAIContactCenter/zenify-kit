package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

// planeMCPPackage pins the Plane MCP server uvx runs.
const planeMCPPackage = "plane-mcp-server==0.3.5"

const planePATKey = "PLANE_API_KEY"

// planeMCPDeps are the process boundaries `zenify plane-mcp` touches; tests replace them.
type planeMCPDeps struct {
	workspace func() (string, bool)
	manifest  func() (*manifest.Manifest, error)
	getenv    func(string) string
	environ   func() []string
	lookPath  func(string) (string, error)
	run       func(name string, args []string, env []string) error
}

func defaultPlaneMCPDeps() planeMCPDeps {
	return planeMCPDeps{
		workspace: func() (string, bool) {
			cwd, _ := os.Getwd()
			ws, _, ok := resolveWorkspace(cwd, "", os.Getenv, os.UserHomeDir)
			return ws, ok
		},
		// The Plane URL receives the developer's PAT, so it comes only from the
		// trusted embedded manifest: no --manifest, no cwd file, no overlay.
		manifest: func() (*manifest.Manifest, error) { return brokerManifest("", "") },
		getenv:   os.Getenv,
		environ:  os.Environ,
		lookPath: func(f string) (string, error) { return lookPath(f) },
		run: func(name string, args []string, env []string) error {
			c := exec.Command(name, args...) //nolint:gosec // G204 -- fixed uvx invocation with the pinned package, no user input
			c.Env = env
			// stdout is the JSON-RPC channel: the child owns it, unwrapped.
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			return c.Run()
		},
	}
}

// planeMCPEnv is base minus every PLANE_* var plus exactly the three the server reads.
func planeMCPEnv(base []string, url, slug, pat string) []string {
	out := make([]string, 0, len(base)+3)
	for _, kv := range base {
		name, _, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(name, "PLANE_") {
			out = append(out, kv)
		}
	}
	return append(out, planePATKey+"="+pat, "PLANE_BASE_URL="+url, "PLANE_WORKSPACE_SLUG="+slug)
}

// planePAT resolves the developer's PAT env-first, then <ws>/.zenify/.env.
func planePAT(getenv func(string) string, ws string) (string, error) {
	f, err := devenv.Load(devenv.Path(ws))
	if err != nil {
		return "", err
	}
	return devenv.Lookup(getenv, f)(planePATKey), nil
}

// planePATSet reports whether the developer has a Plane PAT.
func planePATSet(getenv func(string) string, ws string) bool {
	pat, err := planePAT(getenv, ws)
	return err == nil && pat != ""
}

func newPlaneMCPCmd() *cobra.Command { return newPlaneMCPCmdWith(defaultPlaneMCPDeps()) }

// newPlaneMCPCmdWith builds the stdio MCP server Claude Code spawns. Every kit
// message is a returned error, which main prints to stderr; stdout is never written.
func newPlaneMCPCmdWith(d planeMCPDeps) *cobra.Command {
	return &cobra.Command{
		Use:           "plane-mcp",
		Short:         "Plane MCP server (stdio) bằng PAT của chính dev; Claude Code tự chạy, không gõ tay", //znf:allow-lang
		Hidden:        true,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			m, err := d.manifest()
			if err != nil {
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("zenify plane-mcp: %w", err))
			}
			if m.Plane == nil {
				return exitcode.New(exitcode.BadArgs, errors.New("zenify plane-mcp: manifest không khai báo plane")) //znf:allow-lang
			}
			ws, ok := d.workspace()
			if !ok {
				return errors.New("zenify plane-mcp: chưa có workspace, chạy zenify up trước") //znf:allow-lang
			}
			// The load error names only the file path, never a value.
			pat, err := planePAT(d.getenv, ws)
			if err != nil {
				return fmt.Errorf("zenify plane-mcp: %w", err)
			}
			if pat == "" {
				return fmt.Errorf("zenify plane-mcp: chưa có PLANE_API_KEY. Chạy zenify up --apply trong terminal để dán token (tạo ở %s)", planeTokenURL(m.Plane.URL)) //znf:allow-lang
			}
			if _, err := d.lookPath("uvx"); err != nil {
				return errors.New("zenify plane-mcp: cần uv (brew install uv, hoặc curl -LsSf https://astral.sh/uv/install.sh | sh)") //znf:allow-lang
			}
			runErr := d.run("uvx", []string{"--from", planeMCPPackage, "plane-mcp-server", "stdio"},
				planeMCPEnv(d.environ(), m.Plane.URL, m.Plane.Workspace, pat))
			var ee *exec.ExitError
			if errors.As(runErr, &ee) {
				return exitcode.New(ee.ExitCode(), fmt.Errorf("zenify plane-mcp: plane-mcp-server thoát với mã %d", ee.ExitCode())) //znf:allow-lang
			}
			if runErr != nil {
				return fmt.Errorf("zenify plane-mcp: %w", runErr)
			}
			return nil
		},
	}
}
