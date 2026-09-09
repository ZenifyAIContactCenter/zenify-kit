package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/migrate"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
	"github.com/spf13/cobra"
)

// runMigrate is the testable core. FAIL-OPEN: always returns nil.
func runMigrate(root, toDir string, apply bool, stdout, stderr io.Writer) error {
	r := gitx.ExecRunner()
	repos := workspace.Discover(root, workspace.DefaultMaxDepth, os.ReadDir)
	items := migrate.BuildPlan(root, toDir, repos)

	for _, it := range items {
		fmt.Fprintf(stdout, "  %-7s %s\n", it.Action, it.Name)
		if it.Action == migrate.Refuse || it.Action == migrate.Skip {
			fmt.Fprintf(stderr, "migrate: %s — %s\n", it.Name, it.Reason)
		}
	}

	if !apply {
		n := 0
		for _, it := range items {
			if it.Action == migrate.Move {
				n++
			}
		}
		fmt.Fprintf(stdout, "\n%d repo sẽ move vào %s/. (dry-run — dùng --apply để thực hiện)\n", n, toDir) //znf:allow-lang
		return nil
	}

	// The manifest lives INSIDE the kit repo, not at the workspace root — and the kit repo
	// itself also gets moved in pass 1. So resolve LAZILY (on the first updateYAML call,
	// after every move is done thanks to Apply's 2-pass) then memoize. Structural, NOT a
	// hardcoded kit repo name.
	var manifestPath string
	var manifestErr error
	var resolved bool
	resolveManifest := func() (string, error) {
		if !resolved {
			resolved = true
			manifestPath, manifestErr = findManifest(root)
		}
		return manifestPath, manifestErr
	}
	updateYAML := func(name, newPath string) error {
		p, err := resolveManifest()
		if err != nil {
			return err
		}
		return updateRepoPathInYAML(p, name, newPath)
	}
	io := migrate.ApplyIO{
		ListWT:     func(dir string) ([]string, error) { return gitx.ListWorktrees(r, dir) },
		Move:       os.Rename,
		MkdirAll:   func(d string) error { return os.MkdirAll(d, 0o750) },
		Repair:     func(repoDir, wt string) error { return gitx.RepairWorktree(r, repoDir, wt) },
		Repoint:    repointSymlink,
		UpdateYAML: updateYAML,
		Resolve:    resolvePath,
	}
	for _, note := range migrate.Apply(items, io) {
		fmt.Fprintln(stdout, "  "+note)
	}
	fmt.Fprintln(stdout, "\nXong. Worktree đã được repair; nhớ restart dev server / herdr workspace của các repo đã move (process cũ vẫn trỏ path cũ).") //znf:allow-lang
	return nil
}

// resolvePath resolves a symlink (e.g. macOS /var → /private/var, also the form
// `git worktree list` returns) so prefixes match correctly in migrate.newWorktreePath.
// Cannot resolve → leave as-is (Clean).
func resolvePath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// repointSymlink re-points a worktree's node_modules symlink (deps:symlink) after a repo
// move: if the repo's deps (read at the NEW location) is "symlink" and the worktree's
// node_modules is a symlink pointing at <old-main>/node_modules, re-point it at
// <new-main>/node_modules. Every other case → no-op (no guessing). Fail-open: the error
// is returned so Apply can roll back.
func repointSymlink(wtOld, wtNew, mainOld, mainNew string) error {
	if readDeps(mainNew) != "symlink" {
		return nil
	}
	nm := filepath.Join(wtNew, "node_modules")
	target, err := os.Readlink(nm)
	if err != nil {
		return nil // not a symlink / doesn't exist → nothing to do
	}
	want := filepath.Join(mainOld, "node_modules")
	if target != want && !sameResolvedPath(target, want) {
		return nil // points elsewhere → leave as-is
	}
	if err := os.Remove(nm); err != nil {
		return err
	}
	return os.Symlink(filepath.Join(mainNew, "node_modules"), nm)
}

// sameResolvedPath matches target/want when the raw compare differs: mainOld may be the RAW
// path (it.From) while the node_modules symlink may store an ALREADY-resolved target (or vice
// versa, e.g. a workspace under macOS /var→/private/var). Both resolve and are equal → treat
// as a match; an EvalSymlinks error (target no longer exists) → treat as NOT a match, no panic.
func sameResolvedPath(target, want string) bool {
	rt, errT := filepath.EvalSymlinks(target)
	rw, errW := filepath.EvalSymlinks(want)
	return errT == nil && errW == nil && rt == rw
}

// readDeps reads the "deps" field from <repoDir>/.claude/worktree.json. Unreadable → "".
func readDeps(repoDir string) string {
	b, err := os.ReadFile(filepath.Join(repoDir, ".claude", "worktree.json")) //nolint:gosec // G304 -- path computed internally from workspace state
	if err != nil {
		return ""
	}
	var c struct {
		Deps string `json:"deps"`
	}
	_ = json.Unmarshal(b, &c)
	return c.Deps
}

// findManifest finds repos.yaml on the POST-move layout: scans repos under root, returns the
// absolute path of the first repo with "manifest/repos.yaml". Structural — NOT a hardcoded
// kit repo name.
func findManifest(root string) (string, error) {
	for _, rp := range workspace.Discover(root, workspace.DefaultMaxDepth, os.ReadDir) {
		p := filepath.Join(rp.Path, "manifest", "repos.yaml")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("no repo under %s contains manifest/repos.yaml", root)
}

// updateRepoPathInYAML edits the "    path: <old>" line RIGHT AFTER "  - name: <name>".
// Line-based, touches only the right repo, preserves comments. Not found → returns an error.
func updateRepoPathInYAML(path, name, newPath string) error {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	inRepo := false
	changed := false
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "- name:") {
			inRepo = strings.TrimSpace(strings.TrimPrefix(t, "- name:")) == name
		}
		if inRepo && strings.HasPrefix(t, "path:") {
			indent := ln[:len(ln)-len(strings.TrimLeft(ln, " "))]
			lines[i] = indent + "path: " + newPath
			changed = true
			inRepo = false
		}
	}
	if !changed {
		return fmt.Errorf("no path found for repo %s", name)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600) //nolint:gosec // G703 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
}

func newMigrateCmd() *cobra.Command {
	var root, toDir string
	var apply bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "gom repo vào một thư mục con (dry-run mặc định; move-and-repair: gom được cả repo dirty và repo còn worktree)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == "" {
				root, _ = os.Getwd()
			}
			return runMigrate(root, toDir, apply, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&root, "workspace", "", "thư mục workspace (mặc định cwd)")    //znf:allow-lang
	cmd.Flags().StringVar(&toDir, "to", "repos", "tên thư mục đích gom repo")            //znf:allow-lang
	cmd.Flags().BoolVar(&apply, "apply", false, "thực hiện move (mặc định chỉ dry-run)") //znf:allow-lang
	return cmd
}
