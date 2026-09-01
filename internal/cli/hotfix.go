package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

// resolveHotfixBaseRef ánh xạ chiến lược hotfix của repo → ref cụ thể.
func resolveHotfixBaseRef(repoRoot string, git gitx.Runner) (string, error) {
	c, err := wt.Load(repoRoot)
	if err != nil {
		return "", err
	}
	switch c.HotfixBaseStrategy {
	case "standalone", "staging":
		return "origin/staging", nil
	case "custom":
		if c.HotfixBaseRef == "" {
			return "", fmt.Errorf("baseStrategy=custom nhưng thiếu hotfixBaseRef")
		}
		return c.HotfixBaseRef, nil
	case "release-latest":
		return latestRelease(git, repoRoot)
	default:
		return "origin/staging", nil
	}
}

var releaseRe = regexp.MustCompile(`release([0-9]+)`)

// latestRelease chạy `git -C <root> branch -r`, lọc release<N>, trả origin/release<N> cao nhất.
func latestRelease(git gitx.Runner, repoRoot string) (string, error) {
	if git == nil {
		return "", fmt.Errorf("release-latest cần git runner")
	}
	out, err := git.Run(repoRoot, "branch", "-r")
	if err != nil {
		return "", fmt.Errorf("git branch -r: %w", err)
	}
	best := -1
	for _, line := range strings.Split(string(out), "\n") {
		mm := releaseRe.FindStringSubmatch(line)
		if mm == nil {
			continue
		}
		n, _ := strconv.Atoi(mm[1])
		if n > best {
			best = n
		}
	}
	if best < 0 {
		return "", fmt.Errorf("không thấy branch release* trong %s", repoRoot)
	}
	return fmt.Sprintf("origin/release%d", best), nil
}

func newHotfixCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "hotfix", Short: "trợ giúp hotfix"}
	baseref := &cobra.Command{
		Use:   "baseref <repoPath>",
		Short: "in base ref hotfix đã resolve theo chiến lược trong worktree.json",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := resolveHotfixBaseRef(args[0], gitx.ExecRunner())
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), ref)
			return nil
		},
	}
	cmd.AddCommand(baseref)
	return cmd
}
