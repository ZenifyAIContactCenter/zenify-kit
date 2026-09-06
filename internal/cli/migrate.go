package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/migrate"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
	"github.com/spf13/cobra"
)

// runMigrate là lõi test được. FAIL-OPEN: luôn trả nil.
func runMigrate(root, toDir string, apply bool, stdout, stderr io.Writer) error {
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
		fmt.Fprintf(stdout, "\n%d repo sẽ move vào %s/. (dry-run — dùng --apply để thực hiện)\n", n, toDir)
		return nil
	}

	// Manifest sống BÊN TRONG repo kit, không ở workspace root — và repo kit tự nó
	// cũng bị move trong pass 1. Nên resolve LƯỜI (lần gọi updateYAML đầu tiên, sau
	// khi mọi move đã xong nhờ Apply 2-pass) rồi memoize. Cấu trúc, KHÔNG hardcode tên repo.
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
	for _, note := range migrate.Apply(items, os.Rename, func(d string) error { return os.MkdirAll(d, 0o755) }, updateYAML) {
		fmt.Fprintln(stdout, "  "+note)
	}
	fmt.Fprintln(stdout, "\nXong. Nhớ restart dev server / herdr workspace của các repo đã move.")
	return nil
}

// findManifest tìm repos.yaml trên layout SAU move: quét repo dưới root, trả về đường dẫn
// tuyệt đối của repo đầu tiên có "manifest/repos.yaml". Cấu trúc — KHÔNG hardcode tên repo kit.
func findManifest(root string) (string, error) {
	for _, rp := range workspace.Discover(root, workspace.DefaultMaxDepth, os.ReadDir) {
		p := filepath.Join(rp.Path, "manifest", "repos.yaml")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("không thấy repo nào chứa manifest/repos.yaml dưới %s", root)
}

// updateRepoPathInYAML sửa dòng "    path: <cũ>" NGAY SAU "  - name: <name>".
// Line-based, chỉ đụng đúng repo, giữ nguyên comment. Không tìm thấy → note, không lỗi cứng.
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
		return fmt.Errorf("không thấy path cho repo %s", name)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

func newMigrateCmd() *cobra.Command {
	var root, toDir string
	var apply bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "gom repo vào một thư mục con (dry-run mặc định, refuse repo dirty/còn worktree)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == "" {
				root, _ = os.Getwd()
			}
			return runMigrate(root, toDir, apply, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&root, "workspace", "", "thư mục workspace (mặc định cwd)")
	cmd.Flags().StringVar(&toDir, "to", "repos", "tên thư mục đích gom repo")
	cmd.Flags().BoolVar(&apply, "apply", false, "thực hiện move (mặc định chỉ dry-run)")
	return cmd
}
