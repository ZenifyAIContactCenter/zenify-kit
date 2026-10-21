package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
)

// ghRefreshPackagesCmd builds `gh auth refresh -h github.com -s read:packages`, wired to the
// process stdin/stderr and the given stdout.
func ghRefreshPackagesCmd(stdout io.Writer) *exec.Cmd {
	cmd := exec.Command("gh", "auth", "refresh", "-h", "github.com", "-s", npmTokenScope) //nolint:gosec // G204 -- fixed trusted binary, constant args
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, os.Stderr
	return cmd
}

// ghRefreshPackages returns a closure that runs the refresh command, with gh's stdout sent to
// stdout.
func ghRefreshPackages(stdout io.Writer) func() error {
	return func() error { return ghRefreshPackagesCmd(stdout).Run() }
}

// npmRefresh is the hook tests replace so no cli test execs a real gh.
var npmRefresh = ghRefreshPackages

// npmLogin sets up NPM_TOKEN regardless of workspace repos, then verifies the result.
func npmLogin(w io.Writer, gh ghx.Runner, profile func() (string, bool), interactive bool, refresh func() error) error {
	if err := setupNPMToken(w, gh, interactive, refresh); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	ok, detail := npmTokenStatus(gh, profile)
	if !ok {
		return exitcode.New(exitcode.Fail, errors.New("npm token: "+detail))
	}
	fmt.Fprintln(w, "npm token: ok")
	return nil
}

// npmTokenFix is the doctor Fix for the npm token check. Setup output is discarded so
// `doctor --json --fix` keeps stdout clean.
func npmTokenFix(gh ghx.Runner, profile func() (string, bool), interactive func() bool, refresh func() error) func() (bool, string) {
	return func() (bool, string) {
		if err := setupNPMToken(io.Discard, gh, interactive(), refresh); err != nil {
			return false, err.Error()
		}
		return npmTokenStatus(gh, profile)
	}
}

func newNpmCmd() *cobra.Command {
	c := &cobra.Command{Use: "npm", Short: "Cấu hình npm cho package private trên GitHub Packages"} //znf:allow-lang
	login := &cobra.Command{
		Use:   "login",
		Short: "Đặt NPM_TOKEN từ gh vào shell profile và cấp scope read:packages", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			interactive := npmTokenInteractive(false, term.IsTerminal(int(os.Stdin.Fd())), term.IsTerminal(int(os.Stdout.Fd())))
			return npmLogin(cmd.OutOrStdout(), currentGH{}, currentShellProfile, interactive, npmRefresh(os.Stdout))
		},
	}
	c.AddCommand(login)
	return c
}
