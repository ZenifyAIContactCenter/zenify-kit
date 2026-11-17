package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsgen"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsview"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsweep"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsync"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

// defaultDocsRepo: default docs repo dir name.
// Override with --dir for a different workspace (stays project-agnostic).
const defaultDocsRepo = "docs"

// storeDirs are the docs/ entries linked into the knowledge store; localDirs are
// real directories that stay on this machine. docsDirs is every known entry.
var storeDirs = []string{"specs", "plans", "reference", "handoff", "releases"}
var localDirs = []string{"research", "notes", "scripts", "exports", "external"}
var docsDirs = append(append([]string{}, storeDirs...), localDirs...)

// ensureLocalDirs creates the local dirs under viewDir. Errors become notes
// (fail-open); no error, no notes.
func ensureLocalDirs(viewDir string) []string {
	var notes []string
	for _, d := range localDirs {
		p := filepath.Join(viewDir, d)
		if err := os.MkdirAll(p, 0o755); err != nil { //nolint:gosec // G301 -- docs dirs stay readable like the store view (docsview/view.go)
			notes = append(notes, "docs local: "+p+": "+err.Error())
		}
	}
	return notes
}

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
	for _, n := range ensureLocalDirs(viewDir) {
		u.Note(n)
	}
	return nil
}

// docsSweepCore plans (and with apply, executes) the knowledge-store sweep. Everything goes to
// stderr and it always returns nil (fail-open) so wt sweep can chain it.
func docsSweepCore(workspaceDir, storeDir string, apply, noFetch bool, stderr io.Writer) error {
	if workspaceDir == "" {
		workspaceDir, _ = os.Getwd()
	}
	u := ui.New(stderr)
	store := storeDir // an explicit --dir wins; resolveDocsStore is never consulted
	if store == "" {
		store = resolveDocsStore(workspaceDir, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir)
	}
	if fi, err := os.Stat(filepath.Join(store, "specs")); err != nil || !fi.IsDir() {
		u.Note("docs sweep: store " + store + " không có specs/, bỏ qua") //znf:allow-lang
		return nil
	}
	viewDir := filepath.Join(workspaceDir, defaultDocsRepo)
	if viewDir != store {
		if _, err := os.Stat(viewDir); err != nil { // no view yet: demote next to the store
			viewDir = store
		}
	}
	// Bring the store level with its remote before planning, so --apply never writes onto a stale
	// base (a header or delete there conflicts with a teammate's newer edit on the next sync). It
	// cannot be brought up to date → the whole run is a dry-run (fail-open).
	if apply {
		n, err := docsync.Update(gitx.TimeoutRunner(wt.RemoteTimeout), store)
		if n != "" {
			u.Note(n)
		}
		if err != nil {
			u.Note("docs sweep: store chưa cập nhật được với remote (" + err.Error() + ") — bỏ --apply, chỉ in kế hoạch") //znf:allow-lang
			apply = false
		}
	}
	resolve := func(name string) (string, bool) {
		return workspace.Resolve(workspaceDir, name, workspace.DefaultMaxDepth, os.ReadDir)
	}
	statuses := computeStatuses(store, noFetch, "", resolve, resolveBase, gitx.ExecRunner(), stderr)
	acts, notes := docsweep.Plan(docsweep.OSFS{}, store, statuses, time.Now())
	for _, n := range notes {
		u.Note(n)
	}
	count := map[string]int{}
	for _, a := range acts {
		count[a.Kind]++
		u.Note("docs sweep: " + a.Kind + " " + a.Path + " (" + a.Detail + ")")
	}
	plan, handoff, ref, hdr := count[docsweep.KindDeletePlan], count[docsweep.KindDeleteHandoff], count[docsweep.KindDeleteReference], count[docsweep.KindHeader]
	if !apply {
		u.Note(fmt.Sprintf("docs sweep: %d plan, %d handoff, %d reference, %d header — dry-run, thêm --apply để chạy", plan, handoff, ref, hdr)) //znf:allow-lang
		return nil
	}
	for _, n := range docsweep.Apply(docsweep.OSFS{}, store, viewDir, acts) {
		u.Note(n)
	}
	u.Note(fmt.Sprintf("docs sweep: đã xóa %d file, ghi %d header; bản hạ cấp ở %s/research và %s/notes", plan+handoff+ref, hdr, viewDir, viewDir)) //znf:allow-lang
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
				for _, n := range ensureLocalDirs(viewDir) {
					uErr.Note(n)
				}
				return nil
			}
			return docsSyncCore(workspaceDir, cmd.ErrOrStderr())
		},
	}
	sync.Flags().StringVar(&workspaceDir, "workspace", "", "thư mục workspace (mặc định: tự tìm, không có thì cwd)") //znf:allow-lang
	sync.Flags().StringVar(&dir, "dir", "", "thư mục repo docs (mặc định tự tìm theo layout)")                       //znf:allow-lang
	cmd.AddCommand(sync)

	var sweepWs, sweepDir string
	var sweepApply, sweepNoFetch bool
	sweep := &cobra.Command{
		Use:   "sweep",
		Short: "dọn knowledge store: xóa plan/handoff/reference mồ côi, ghi header trạng thái spec (mặc định dry-run)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if sweepWs == "" {
				sweepWs = workspaceOrCwd("", cmd.ErrOrStderr())
			}
			return docsSweepCore(sweepWs, sweepDir, sweepApply, sweepNoFetch, cmd.ErrOrStderr())
		},
	}
	sweep.Flags().BoolVar(&sweepApply, "apply", false, "thực hiện thay đổi (mặc định chỉ in kế hoạch)")          //znf:allow-lang
	sweep.Flags().BoolVar(&sweepNoFetch, "no-fetch", false, "bỏ git fetch khi tính trạng thái spec")             //znf:allow-lang
	sweep.Flags().StringVar(&sweepWs, "workspace", "", "thư mục workspace (mặc định: tự tìm, không có thì cwd)") //znf:allow-lang
	sweep.Flags().StringVar(&sweepDir, "dir", "", "knowledge store cần dọn (mặc định: tự tìm như docs sync)")    //znf:allow-lang
	cmd.AddCommand(sweep)

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
			skills, err := docsgen.GenSkills(plugin.ZnfFS())
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
