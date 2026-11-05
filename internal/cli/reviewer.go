package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/spf13/cobra"
)

// decideReviewer picks the review engine: codex only when the session asks for
// it (running inside Codex, or ZNF_REVIEWER=codex) and the binary is on PATH.
func decideReviewer(getenv func(string) string, onPath bool) (engine, reason string) {
	wantsCodex := getenv("CODEX_THREAD_ID") != "" || getenv(apply.ReviewerEnv) == "codex"
	switch {
	case wantsCodex && onPath:
		return "codex", ""
	case wantsCodex:
		return "agent", "codex không có trên PATH" //znf:allow-lang
	default:
		return "agent", "ZNF_REVIEWER=agent"
	}
}

func printReviewer(w io.Writer, engine, reason string) {
	if reason == "" {
		fmt.Fprintln(w, engine)
		return
	}
	fmt.Fprintf(w, "%s (%s)\n", engine, reason)
}

func newReviewerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reviewer",
		Short: "In engine review được chọn (agent hoặc codex) và lý do", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			engine, reason := decideReviewer(os.Getenv, codexOnPath())
			printReviewer(cmd.OutOrStdout(), engine, reason)
			return nil
		},
	}
	cmd.AddCommand(newReviewerSetCmd(), newReviewerGetCmd())
	return cmd
}

// reviewerSettings is the file holding env.ZNF_REVIEWER: the resolved
// workspace's .claude/settings.local.json, where ensure writes it (FR-8.2).
func reviewerSettings() (string, error) {
	cwd, _ := os.Getwd()
	ws, _, ok := resolveWorkspace(cwd, "", os.Getenv, os.UserHomeDir)
	if !ok {
		return "", errors.New("chưa có workspace: chạy lệnh trong workspace hoặc chạy zenify up trước") //znf:allow-lang
	}
	return apply.WorkspaceLocalSettings(ws), nil
}

// newReviewerSetCmd writes env.ZNF_REVIEWER of <ws>/.claude/settings.local.json (FR-16.4);
// a session reads it at start, so the change applies to new sessions.
func newReviewerSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <agent|codex>",
		Short: "Đặt engine review (agent hoặc codex) vào env của <workspace>/.claude/settings.local.json", //znf:allow-lang
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := reviewerSettings()
			if err != nil {
				return err
			}
			if err := apply.SetReviewerEnv(path, args[0], false); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", apply.ReviewerEnv, args[0])
			return nil
		},
	}
}

// newReviewerGetCmd prints the stored env.ZNF_REVIEWER of <ws>/.claude/settings.local.json — the
// value `reviewer set` writes — which the /config row follows (FR-17.3).
func newReviewerGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "In engine review đã lưu trong env của <workspace>/.claude/settings.local.json (agent hoặc codex)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := reviewerSettings()
			if err != nil {
				return err
			}
			v, err := apply.ReviewerEnvValue(path)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), v)
			return err
		},
	}
}
