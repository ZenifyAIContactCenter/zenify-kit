package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsgen"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsview"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsync"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/spf13/cobra"
)

// defaultDocsRepo: default docs repo dir name.
// Override with --dir for a different workspace (stays project-agnostic).
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
	u := ui.New(errW)
	dir := resolveDocsStore(workspace, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir)
	for _, n := range docsync.Sync(gitx.ExecRunner(), dir) {
		u.Note(n)
	}
	// view link farm — runs on every OS (unix symlink / windows junction, wrapped in OSFS)
	viewDir := filepath.Join(workspace, defaultDocsRepo)
	if viewDir != dir { // only when the store HAS separated from the workspace (already migrated)
		for _, n := range docsview.EnsureView(docsview.OSFS{}, dir, viewDir) {
			u.Note(n)
		}
	}
	return nil
}

func newDocsCmd() *cobra.Command {
	var workspaceDir, dir string
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "quản lý docs layer (agent-managed, dev read-only)", //znf:allow-lang
	}
	sync := &cobra.Command{
		Use:   "sync",
		Short: "đồng bộ docs: pull + commit + push (fail-open)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if workspaceDir == "" {
				workspaceDir = workspaceOrCwd("", cmd.ErrOrStderr())
			}
			// --dir override wins over resolveDocsStore, preserving the old behavior:
			// only when --dir is NOT set does it auto-resolve in the core.
			if dir != "" {
				uErr := uiErr(cmd)
				for _, n := range docsync.Sync(gitx.ExecRunner(), dir) {
					uErr.Note(n)
				}
				viewDir := filepath.Join(workspaceDir, defaultDocsRepo)
				if viewDir != dir {
					for _, n := range docsview.EnsureView(docsview.OSFS{}, dir, viewDir) {
						uErr.Note(n)
					}
				}
				return nil
			}
			return docsSyncCore(workspaceDir, cmd.ErrOrStderr())
		},
	}
	sync.Flags().StringVar(&workspaceDir, "workspace", "", "thư mục workspace (mặc định: tự tìm, không có thì cwd)") //znf:allow-lang
	sync.Flags().StringVar(&dir, "dir", "", "thư mục repo docs (mặc định tự tìm theo layout)")                       //znf:allow-lang
	cmd.AddCommand(sync)

	var genOut string
	var genCheck bool
	gen := &cobra.Command{
		Use:   "gen",
		Short: "sinh reference cho site docs (CLI từ cobra, skill/agent từ frontmatter, bảng hook)", //znf:allow-lang
		Long: "Ghi markdown VitePress vào --out (mặc định website/reference). " + //znf:allow-lang
			"Với --check không ghi, chỉ so với file trên đĩa; lệch → exit 1 và in danh sách. CI chạy --check.", //znf:allow-lang
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			files, err := docsgen.GenCLI(NewRootCmd())
			if err != nil {
				return err
			}
			skills, err := docsgen.GenSkills(plugin.ZnfFS(), plugin.CodingFS())
			if err != nil {
				return err
			}
			for k, v := range skills {
				files[k] = v
			}
			for k, v := range docsgen.GenHooks(apply.HookSpecs()) {
				files[k] = v
			}
			uErr := uiErr(cmd)
			if genCheck {
				if diff := docsgen.Check(genOut, files); len(diff) > 0 {
					for _, d := range diff {
						uErr.Step(ui.StatusFail, "docs gen --check: lệch "+d, "") //znf:allow-lang
					}
					return exitcode.New(exitcode.Fail,
						fmt.Errorf("docs gen --check: %d file lệch — chạy `zenify docs gen` rồi commit", len(diff))) //znf:allow-lang
				}
				uErr.Step(ui.StatusOK, "docs gen --check: khớp.", "") //znf:allow-lang
				return nil
			}
			if err := docsgen.Write(genOut, files); err != nil {
				return err
			}
			uErr.Step(ui.StatusOK, fmt.Sprintf("docs gen: %d file → %s", len(files), genOut), "") //znf:allow-lang
			return nil
		},
	}
	gen.Flags().StringVar(&genOut, "out", filepath.Join("website", "reference"), "thư mục đích") //znf:allow-lang
	gen.Flags().BoolVar(&genCheck, "check", false, "chỉ so, không ghi; lệch → exit 1")           //znf:allow-lang
	cmd.AddCommand(gen)

	return cmd
}
