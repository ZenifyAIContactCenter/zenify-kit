package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitguard"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/secretscan"
	"github.com/spf13/cobra"
)

type hookPayload struct {
	Cwd       string `json:"cwd"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

// urlHosts returns the lower-cased hosts in the authority of URL v, one per
// comma-separated entry (replica-set URIs list several); nil when v has no
// scheme. Ports and IPv6 brackets are stripped.
func urlHosts(v string) []string {
	_, rest, ok := strings.Cut(v, "://")
	if !ok {
		return nil
	}
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest = rest[:j]
	}
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		rest = rest[at+1:]
	}
	var out []string
	for _, h := range strings.Split(rest, ",") {
		if strings.HasPrefix(h, "[") {
			h, _, _ = strings.Cut(h[1:], "]")
		} else {
			h, _, _ = strings.Cut(h, ":")
		}
		if h != "" {
			out = append(out, strings.ToLower(h))
		}
	}
	return out
}

// prodHosts lists the distinct non-loopback hostnames of every *_PRD key in f.
func prodHosts(f *devenv.File) []gitguard.ProdHost {
	var out []gitguard.ProdHost
	seen := map[string]bool{}
	for _, k := range f.Keys() {
		if !strings.HasSuffix(k, "_PRD") {
			continue
		}
		v, _ := f.Get(k)
		for _, h := range urlHosts(v) {
			if h == "localhost" || h == "::1" || h == "0.0.0.0" || seen[h] {
				continue
			}
			if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
				continue
			}
			seen[h] = true
			out = append(out, gitguard.ProdHost{Key: k, Host: h})
		}
	}
	return out
}

// guardOptions resolves the workspace from cwd and reads its _PRD hosts.
// Anything missing or unreadable yields fewer options, never an error.
func guardOptions(cwd string, getenv func(string) string) gitguard.Options {
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	ws, _, ok := resolveWorkspace(cwd, "", getenv, os.UserHomeDir)
	if !ok {
		return gitguard.Options{}
	}
	path := devenv.Path(ws)
	f, err := devenv.Load(path)
	if err != nil {
		return gitguard.Options{EnvFile: path}
	}
	return gitguard.Options{EnvFile: path, ProdHosts: prodHosts(f)}
}

// decideFromPayload is the pure core: parse the hook payload, call
// gitguard.Decide with an onCommit callback wired to secretscan. Any JSON
// error → fail-open (empty Decision).
func decideFromPayload(payload []byte, getenv func(string) string) gitguard.Decision {
	var p hookPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return gitguard.Decision{}
	}
	if p.ToolInput.Command == "" {
		return gitguard.Decision{}
	}
	onCommit := func(repoDir string) gitguard.Decision {
		s, err := secretscan.New()
		if err != nil {
			return gitguard.Decision{} // scanner init failed → fail-open
		}
		deny, msg, err := secretscan.Staged(repoDir, s)
		if err != nil || !deny {
			return gitguard.Decision{}
		}
		return gitguard.Decision{Deny: true, Message: msg}
	}
	return gitguard.Decide(p.ToolInput.Command, p.Cwd, getenv, onCommit, guardOptions(p.Cwd, getenv))
}

// runGitGuard is the process-exit-code core, factored out of RunE so tests
// can inject a decide func that panics without exec'ing the built binary.
// A deferred recover() guarantees a panic anywhere in decide (gitguard.Decide
// / secretscan) falls open to allow (0), never to 2 — a Go panic's default
// exit code is also 2, which the hook contract reads as a deliberate DENY.
// The recovered value is never printed: it could echo payload/secret text
// (FR-041), so only a generic diagnostic goes to stderr.
func runGitGuard(stdin io.Reader, stderr io.Writer, getenv func(string) string, decide func([]byte, func(string) string) gitguard.Decision) (code int) {
	defer func() {
		if r := recover(); r != nil {
			_, _ = fmt.Fprintln(stderr, "🚫 [git-guard] internal error — failing open (allow)")
			code = 0
		}
	}()
	payload, err := io.ReadAll(stdin)
	if err != nil {
		return 0 // fail-open
	}
	d := decide(payload, getenv)
	if d.Deny {
		_, _ = fmt.Fprintln(stderr, d.Message)
		return 2
	}
	return 0
}

func newGitGuardCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "git-guard",
		Short:  "Hook PreToolUse: chặn commit/merge/push vào deploy branch + secret đã stage", //znf:allow-lang
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			code := runGitGuard(cmd.InOrStdin(), cmd.ErrOrStderr(), os.Getenv, decideFromPayload)
			if code != 0 {
				os.Exit(code) // DENY per Claude Code hook contract — NOT via exitcode package
			}
			return nil // allow (exit 0)
		},
	}
}
