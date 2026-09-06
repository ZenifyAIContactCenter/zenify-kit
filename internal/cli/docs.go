package cli

import (
	"os"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsync"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/spf13/cobra"
)

// defaultDocsRepo: tên dir repo docs mặc định (Task 7 đổi sang "docs").
// Override bằng --dir cho workspace khác (giữ project-agnostic).
const defaultDocsRepo = "zenify-knowledge"

func newDocsCmd() *cobra.Command {
	var workspaceDir, dir string
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "quản lý docs layer (agent-managed, dev read-only)",
	}
	sync := &cobra.Command{
		Use:   "sync",
		Short: "đồng bộ docs: pull + commit + push (fail-open)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if workspaceDir == "" {
				workspaceDir, _ = os.Getwd()
			}
			if dir == "" {
				dir = resolveWorkspaceRepoDir(workspaceDir, defaultDocsRepo, "", os.ReadDir)
			}
			for _, n := range docsync.Sync(gitx.ExecRunner(), dir) {
				cmd.PrintErrln(n)
			}
			return nil
		},
	}
	sync.Flags().StringVar(&workspaceDir, "workspace", "", "thư mục workspace (mặc định cwd)")
	sync.Flags().StringVar(&dir, "dir", "", "thư mục repo docs (mặc định tự tìm theo layout)")
	cmd.AddCommand(sync)
	return cmd
}
