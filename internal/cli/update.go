package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/update"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
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
