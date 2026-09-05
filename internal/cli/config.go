package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/distribute"
	"github.com/spf13/cobra"
)

// defaultConfigRepo/Sub: config dir mặc định = <workspace>/zenify-knowledge/config
// (quy ước record-layer M6a). Override bằng --config-dir cho workspace khác.
const defaultConfigRepo = "zenify-knowledge"
const defaultConfigSub = "config"

// runConfig là lõi test được. FAIL-OPEN: luôn trả nil; lỗi thành note ra stderr.
func runConfig(workspace, configDir string, apply bool, stdout, stderr io.Writer) error {
	if configDir == "" {
		configDir = filepath.Join(workspace, defaultConfigRepo, defaultConfigSub)
	}
	manifestPath := filepath.Join(configDir, "distribution.txt")
	mb, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "config: không đọc được manifest %s: %v (fail-open)\n", manifestPath, err)
		return nil
	}
	pairs, notes := distribute.ParseManifest(mb)
	for _, n := range notes {
		fmt.Fprintln(stderr, "config: "+n)
	}
	readSource := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(configDir, rel)) }
	readDest := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(workspace, rel)) }
	plans := distribute.Plan(pairs, readSource, readDest)

	fmt.Fprintf(stdout, "config dir: %s\n\n", configDir)
	var nCreate, nUpdate, nSame, nSkip int
	for _, p := range plans {
		fmt.Fprintf(stdout, "  %-7s %s → %s\n", p.State, p.Source, p.Dest)
		switch p.State {
		case distribute.Create:
			nCreate++
		case distribute.Update:
			nUpdate++
			fmt.Fprintln(stdout, indentBlock(p.Diff))
		case distribute.Same:
			nSame++
		case distribute.Skip:
			nSkip++
			fmt.Fprintf(stderr, "config: bỏ %s — không đọc được nguồn\n", p.Source)
		}
	}

	if apply {
		writeDest := func(rel string, data []byte) error {
			full := filepath.Join(workspace, rel)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return err
			}
			return os.WriteFile(full, data, 0o644)
		}
		for _, n := range distribute.Apply(plans, readSource, writeDest) {
			fmt.Fprintln(stdout, "  "+n)
		}
		fmt.Fprintf(stdout, "\nĐã áp dụng: %d tạo, %d cập nhật (%d giữ nguyên, %d bỏ).\n", nCreate, nUpdate, nSame, nSkip)
		return nil
	}
	if nCreate+nUpdate == 0 {
		fmt.Fprintln(stdout, "\nTất cả đã đồng bộ. (dry-run — dùng --apply để ghi)")
	} else {
		fmt.Fprintf(stdout, "\n%d CREATE, %d UPDATE, %d SAME. (dry-run — dùng --apply để ghi)\n", nCreate, nUpdate, nSame)
	}
	return nil
}

// indentBlock thụt mỗi dòng của khối diff 4 khoảng trắng.
func indentBlock(s string) string {
	if s == "" {
		return ""
	}
	return "    " + strings.ReplaceAll(s, "\n", "\n    ")
}

func newConfigCmd() *cobra.Command {
	var workspace, configDir string
	var apply bool
	cmd := &cobra.Command{
		Use:   "config",
		Short: "phân phối config workspace-level từ zenify-knowledge/config (dry-run mặc định)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if workspace == "" {
				workspace, _ = os.Getwd()
			}
			return runConfig(workspace, configDir, apply, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "thư mục workspace (mặc định cwd)")
	cmd.Flags().StringVar(&configDir, "config-dir", "", "thư mục config nguồn (mặc định <workspace>/zenify-knowledge/config)")
	cmd.Flags().BoolVar(&apply, "apply", false, "ghi thay đổi (mặc định chỉ dry-run)")
	return cmd
}
