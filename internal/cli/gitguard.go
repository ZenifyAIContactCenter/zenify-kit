package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

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
	return gitguard.Decide(p.ToolInput.Command, p.Cwd, getenv, onCommit)
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
			fmt.Fprintln(stderr, "🚫 [git-guard] internal error — failing open (allow)")
			code = 0
		}
	}()
	payload, err := io.ReadAll(stdin)
	if err != nil {
		return 0 // fail-open
	}
	d := decide(payload, getenv)
	if d.Deny {
		fmt.Fprintln(stderr, d.Message)
		return 2
	}
	return 0
}

func newGitGuardCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "git-guard",
		Short:  "PreToolUse hook: block commit/merge/push to deploy branches + staged secrets",
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
