package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsview"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsync"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/spf13/cobra"
)

// defaultDocsRepo: tên dir repo docs mặc định.
// Override bằng --dir cho workspace khác (giữ project-agnostic).
const defaultDocsRepo = "docs"

// docsSyncCore is the callable core of `docs sync`, extracted so hook
// dispatch (hookactions.go) can invoke it directly without exec'ing the
// binary. It resolves the docs store from workspace (empty => cwd), then
// syncs and ensures the view link farm — identical behavior to the RunE
// body it replaced. Sync notes go to errW (stderr in the subcommand).
func docsSyncCore(workspace string, errW io.Writer) error {
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	dir := resolveDocsStore(workspace, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir)
	for _, n := range docsync.Sync(gitx.ExecRunner(), dir) {
		fmt.Fprintln(errW, n)
	}
	// view link farm — chạy mọi OS (unix symlink / windows junction, gói trong OSFS)
	viewDir := filepath.Join(workspace, defaultDocsRepo)
	if viewDir != dir { // chỉ khi store ĐÃ tách khỏi workspace (đã migrate)
		for _, n := range docsview.EnsureView(docsview.OSFS{}, dir, viewDir) {
			fmt.Fprintln(errW, n)
		}
	}
	return nil
}

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
			// --dir override thắng resolveDocsStore, giữ nguyên hành vi cũ:
			// chỉ khi --dir KHÔNG được set thì mới tự resolve trong core.
			if dir != "" {
				for _, n := range docsync.Sync(gitx.ExecRunner(), dir) {
					cmd.PrintErrln(n)
				}
				viewDir := filepath.Join(workspaceDir, defaultDocsRepo)
				if viewDir != dir {
					for _, n := range docsview.EnsureView(docsview.OSFS{}, dir, viewDir) {
						cmd.PrintErrln(n)
					}
				}
				return nil
			}
			return docsSyncCore(workspaceDir, cmd.ErrOrStderr())
		},
	}
	sync.Flags().StringVar(&workspaceDir, "workspace", "", "thư mục workspace (mặc định cwd)")
	sync.Flags().StringVar(&dir, "dir", "", "thư mục repo docs (mặc định tự tìm theo layout)")
	cmd.AddCommand(sync)
	return cmd
}
