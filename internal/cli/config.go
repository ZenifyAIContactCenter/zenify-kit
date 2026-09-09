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

// defaultConfigSub: config dir mặc định = store docs (resolveDocsStore), subdir ẩn .config
// (docs layer agent-managed; config ẩn để dev chỉ thấy spec/plan/handoff/release).
// Store tự tìm qua $ZENIFY_HOME/~/.zenify/knowledge/workspace fallback (Task 1).
// Override bằng --config-dir cho workspace khác.
const defaultConfigSub = ".config"

// runConfig là lõi test được. FAIL-OPEN: luôn trả nil; lỗi thành note ra stderr.
func runConfig(workspace, configDir string, apply bool, stdout, stderr io.Writer) error {
	if configDir == "" {
		configDir = filepath.Join(resolveDocsStore(workspace, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir), defaultConfigSub)
	}
	manifestPath := filepath.Join(configDir, "distribution.txt")
	mb, err := os.ReadFile(manifestPath) //nolint:gosec // G304 -- manifestPath is inside the trusted config dir, not user input
	if err != nil {
		fmt.Fprintf(stderr, "config: không đọc được manifest %s: %v (fail-open)\n", manifestPath, err)
		return nil
	}
	pairs, notes := distribute.ParseManifest(mb)
	listDir := func(rel string) ([]string, error) {
		ents, err := os.ReadDir(filepath.Join(configDir, rel))
		if err != nil {
			return nil, err
		}
		var names []string
		for _, e := range ents {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				names = append(names, e.Name())
			}
		}
		return names, nil
	}
	pairs, dnotes := distribute.ExpandDirPairs(pairs, listDir)
	notes = append(notes, dnotes...)
	for _, n := range notes {
		fmt.Fprintln(stderr, "config: "+n)
	}
	readSource := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(configDir, rel)) } //nolint:gosec // G304 -- rel comes from the trusted manifest, joined under configDir
	readDest := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(workspace, rel)) }   //nolint:gosec // G304 -- rel comes from the trusted manifest, joined under the workspace
	plans := distribute.Plan(pairs, readSource, readDest, os.IsNotExist)

	fmt.Fprintf(stdout, "config dir: %s\n\n", configDir)
	var nSame, nSkip int
	for _, p := range plans {
		fmt.Fprintf(stdout, "  %-7s %s → %s\n", p.State, p.Source, p.Dest)
		switch p.State {
		case distribute.Update:
			fmt.Fprintln(stdout, indentBlock(p.Diff))
		case distribute.Same:
			nSame++
		case distribute.Skip:
			nSkip++
			fmt.Fprintf(stderr, "config: bỏ %s → %s — %s\n", p.Source, p.Dest, p.Reason)
		}
	}

	if apply {
		writeDest := func(rel string, data []byte) error {
			full := filepath.Join(workspace, rel)
			if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
				return err
			}
			return os.WriteFile(full, data, 0o600)
		}
		nWritten := 0
		for _, n := range distribute.Apply(plans, readSource, writeDest) {
			fmt.Fprintln(stdout, "  "+n)
			if strings.HasPrefix(n, "đã ghi ") {
				nWritten++
			}
		}
		fmt.Fprintf(stdout, "\nĐã áp dụng: %d ghi (%d giữ nguyên, %d bỏ).\n", nWritten, nSame, nSkip)
		return nil
	}

	nChange := 0
	for _, p := range plans {
		if p.State == distribute.Create || p.State == distribute.Update {
			nChange++
		}
	}
	if len(plans) == 0 {
		fmt.Fprintln(stdout, "\nManifest trống — không có cặp nào để phân phối.")
	} else if nChange == 0 {
		fmt.Fprintln(stdout, "\nTất cả đã đồng bộ. (dry-run — dùng --apply để ghi)")
	} else {
		fmt.Fprintf(stdout, "\n%d thay đổi, %d giữ nguyên, %d bỏ. (dry-run — dùng --apply để ghi)\n", nChange, nSame, nSkip)
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
		Short: "phân phối config workspace-level từ docs/.config (dry-run mặc định)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if workspace == "" {
				workspace, _ = os.Getwd()
			}
			return runConfig(workspace, configDir, apply, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "thư mục workspace (mặc định cwd)")
	cmd.Flags().StringVar(&configDir, "config-dir", "", "thư mục config nguồn (mặc định repo docs/.config, tự tìm theo layout)")
	cmd.Flags().BoolVar(&apply, "apply", false, "ghi thay đổi (mặc định chỉ dry-run)")
	return cmd
}
