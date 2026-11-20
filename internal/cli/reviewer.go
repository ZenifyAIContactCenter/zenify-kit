package cli

// znf:ui-verify-ok: the kit renders no frontend; the ship regex "views?/" matches skills/review/.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/spf13/cobra"
)

// decideReviewer picks the review engine. An unset ZNF_REVIEWER is the kit default
// (apply.ReviewerDefault). project (or agent+project) wins when a CLAUDE.md declares a Pre-PR
// review line (it beats CODEX_THREAD_ID); project without one falls back to agent. agent+project
// without one behaves exactly as agent: codex only when the session asks for it (running inside
// Codex, or ZNF_REVIEWER=codex) and the binary is on PATH, else agent with the Pre-PR reason.
func decideReviewer(getenv func(string) string, onPath bool, prePR bool) (engine, reason string) {
	zr := getenv(apply.ReviewerEnv)
	if zr == "" {
		zr = apply.ReviewerDefault
	}
	noLine := "không có dòng Pre-PR review" //znf:allow-lang
	if zr == "project" || zr == "agent+project" {
		if prePR {
			return zr, ""
		}
		if zr == "project" {
			return "agent", noLine
		}
	}
	wantsCodex := getenv("CODEX_THREAD_ID") != "" || zr == "codex"
	switch {
	case wantsCodex && onPath:
		return "codex", ""
	case zr == "agent+project":
		return "agent", noLine
	case wantsCodex:
		return "agent", "codex không có trên PATH" //znf:allow-lang
	default:
		return "agent", "ZNF_REVIEWER=agent"
	}
}

// hasPrePRLine reports whether a CLAUDE.md from cwd up to / or home/.claude/CLAUDE.md
// has a line starting "Pre-PR review:".
func hasPrePRLine(cwd, home string, read func(string) ([]byte, error)) bool {
	has := func(path string) bool {
		b, err := read(path)
		if err != nil {
			return false
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "Pre-PR review:") {
				return true
			}
		}
		return false
	}
	for dir := filepath.Clean(cwd); ; dir = filepath.Dir(dir) {
		if has(filepath.Join(dir, "CLAUDE.md")) {
			return true
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return has(filepath.Join(home, ".claude", "CLAUDE.md"))
}

// prePRDeclared is hasPrePRLine over the real filesystem from the current directory.
func prePRDeclared() bool {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	return hasPrePRLine(cwd, home, os.ReadFile)
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
		Short: "In engine review được chọn (agent, codex, project hoặc agent+project) và lý do", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			engine, reason := decideReviewer(os.Getenv, codexOnPath(), prePRDeclared())
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
		Use:   "set <agent|codex|project|agent+project>",
		Short: "Đặt engine review (agent, codex, project hoặc agent+project) vào env của <workspace>/.claude/settings.local.json", //znf:allow-lang
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := reviewerSettings()
			if err != nil {
				return err
			}
			if err := apply.SetReviewerEnv(path, args[0], false); err != nil {
				return err
			}
			// An explicit choice must survive the one-time default migration.
			if home, herr := os.UserHomeDir(); herr == nil {
				if err := apply.RecordReviewerDefaultMigrated(path, apply.KitReviewerDefaultRecord(home)); err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "zenify reviewer set:", err)
				}
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
		Short: "In engine review đã lưu trong env của <workspace>/.claude/settings.local.json (agent, codex, project hoặc agent+project)", //znf:allow-lang
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
