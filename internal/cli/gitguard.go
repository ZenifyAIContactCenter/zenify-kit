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

func newGitGuardCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "git-guard",
		Short:  "PreToolUse hook: block commit/merge/push to deploy branches + staged secrets",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return nil // fail-open
			}
			d := decideFromPayload(payload, os.Getenv)
			if d.Deny {
				fmt.Fprintln(cmd.ErrOrStderr(), d.Message)
				os.Exit(2) // DENY per Claude Code hook contract — NOT via exitcode package
			}
			return nil // allow (exit 0)
		},
	}
}
