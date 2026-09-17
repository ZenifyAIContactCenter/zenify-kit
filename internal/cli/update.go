package cli

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/update"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
	"github.com/spf13/cobra"
)

// updateCheckTimeout bounds every network touch of the update check — the
// session-start hook must never hold a session open waiting on github.com.
const updateCheckTimeout = 2 * time.Second

func updateClient() *http.Client { return &http.Client{Timeout: updateCheckTimeout} }

// updateOptions is the one place the hook and `zenify update` agree on where
// the cache lives and which URL to ask. ZENIFY_UPDATE_URL exists for tests
// and for pointing a machine at a mirror.
func updateOptions(force bool) update.Options {
	url := os.Getenv("ZENIFY_UPDATE_URL")
	if url == "" {
		url = update.DefaultURL
	}
	return update.Options{
		Current:  version.Current(),
		CacheDir: zenifyHome(os.Getenv, os.UserHomeDir),
		URL:      url,
		Client:   updateClient(),
		Force:    force,
	}
}

// detectMethod classifies the running binary. brew installs a symlink in
// /opt/homebrew/bin, so the path is resolved first; on any error the raw path
// is classified instead (worst case: Unknown, which only changes the hint).
func detectMethod() update.Method {
	exe, err := os.Executable()
	if err != nil {
		return update.Unknown
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	home, _ := os.UserHomeDir()
	return update.Detect(exe, runtime.GOOS, home, os.Getenv("LOCALAPPDATA"))
}

// upgradeDisplay is the one-line upgrade hint for the running install method.
func upgradeDisplay() string {
	_, _, display := update.Command(detectMethod(), runtime.GOOS)
	return display
}

// updateNudge prints exactly one line to w when a newer release exists.
// Silent on dev builds, on ZENIFY_NO_UPDATE_CHECK, on an up-to-date binary,
// and on any error — session-start stdout is agent context, and a failed
// check is not something the agent should act on.
func updateNudge(w io.Writer) {
	cur := version.Current()
	if cur == "dev" || os.Getenv("ZENIFY_NO_UPDATE_CHECK") != "" {
		return
	}
	res := update.Check(updateOptions(false))
	if res.Err != nil || !res.Newer {
		return
	}
	fmt.Fprintf(w, "zenify: %s is available (running %s) — upgrade: %s\n", res.Latest, cur, upgradeDisplay())
}

func newUpdateCmd() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Nâng zenify lên bản release mới nhất (brew / scoop / install script)", //znf:allow-lang
		Long: "Tự nhận cách binary này được cài rồi chạy lệnh nâng cấp tương ứng:\n" + //znf:allow-lang
			"  brew            brew upgrade --cask zenify\n" +
			"  scoop           scoop update zenify\n" +
			"  install script  chạy lại scripts/install.sh (install.ps1 trên Windows)\n" + //znf:allow-lang
			"\n" +
			"Với --check chỉ báo có bản mới hay không, không cài.", //znf:allow-lang
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			method := detectMethod()
			if check {
				return runUpdateCheck(cmd.OutOrStdout(), cmd.ErrOrStderr(), updateOptions(true), runtime.GOOS, method)
			}
			run := func(name string, args []string) error {
				c := exec.Command(name, args...) //nolint:gosec // G204 -- name/args come from update.Command's fixed table (brew/scoop/sh/powershell), never from user input
				c.Stdin, c.Stdout, c.Stderr = os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr()
				return c.Run()
			}
			return runUpdate(cmd.ErrOrStderr(), method, runtime.GOOS, run, zenifyHome(os.Getenv, os.UserHomeDir))
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "only report whether a newer release exists")
	return cmd
}

// runUpdateCheck prints one status line. Force is expected in opts so the
// user sees the live answer, not yesterday's cache.
func runUpdateCheck(stdout, stderr io.Writer, opts update.Options, goos string, method update.Method) error {
	res := update.Check(opts)
	if res.Err != nil {
		ui.New(stderr).Step(ui.StatusFail, "update check failed", res.Err.Error())
		return exitcode.New(exitcode.Fail, res.Err)
	}
	u := ui.New(stdout)
	if opts.Current == "dev" {
		u.Step(ui.StatusInfo, fmt.Sprintf("zenify dev build — latest %s", res.Latest), "")
		return nil
	}
	if res.Newer {
		_, _, display := update.Command(method, goos)
		u.Step(ui.StatusWarn, fmt.Sprintf("zenify %s — latest %s — upgrade: %s", opts.Current, res.Latest, display), "")
		return nil
	}
	u.Step(ui.StatusOK, fmt.Sprintf("zenify %s — up to date", opts.Current), "")
	return nil
}

// runUpdate executes the upgrade for method through run, then drops the
// check cache so the next session re-reads the installed version. Unknown
// prints every documented path and fails without executing anything.
func runUpdate(stderr io.Writer, method update.Method, goos string, run func(name string, args []string) error, cacheDir string) error {
	u := ui.New(stderr)
	name, args, display := update.Command(method, goos)
	if name == "" {
		u.Step(ui.StatusFail, "zenify: could not tell how this binary was installed — upgrade with one of:", "")
		_, _, brew := update.Command(update.Brew, goos)
		_, _, scoop := update.Command(update.Scoop, goos)
		_, _, script := update.Command(update.Script, goos)
		u.KV([][2]string{
			{"brew", brew},
			{"scoop", scoop},
			{"script", script},
			{"docs", update.InstallDoc},
		})
		return exitcode.New(exitcode.Fail, errors.New("update: unknown install method"))
	}
	u.Step(ui.StatusActive, fmt.Sprintf("zenify: upgrading via %s (%s)", method, display), "")
	if err := run(name, args); err != nil {
		return fmt.Errorf("update: %s failed: %w", display, err)
	}
	if cacheDir != "" {
		if err := os.Remove(filepath.Join(cacheDir, update.CacheFile)); err != nil && !errors.Is(err, os.ErrNotExist) { //nolint:gosec // G703 -- path is <kit home>/update-check.json, the kit's own cache file
			u.Note("zenify: note: could not clear update cache: " + err.Error())
		}
	}
	return nil
}
