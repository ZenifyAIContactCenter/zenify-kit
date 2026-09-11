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

// defaultConfigSub: default config dir = docs store (resolveDocsStore), hidden .config subdir
// (docs layer is agent-managed; config is hidden so devs only see spec/plan/handoff/release).
// Store auto-resolves via $ZENIFY_HOME/~/.zenify/knowledge/workspace fallback (Task 1).
// Override with --config-dir for a different workspace.
const defaultConfigSub = ".config"

// runConfig is the testable core. FAIL-OPEN: always returns a nil error; errors become a note to stderr.
// The int return is the number of files written (0 outside --apply), so callers like
// ensureWorkspace can stay silent when nothing changed.
func runConfig(workspace, configDir string, apply bool, stdout, stderr io.Writer) (int, error) {
	if configDir == "" {
		configDir = filepath.Join(resolveDocsStore(workspace, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir), defaultConfigSub)
	}
	manifestPath := filepath.Join(configDir, "distribution.txt")
	mb, err := os.ReadFile(manifestPath) //nolint:gosec // G304 -- manifestPath is inside the trusted config dir, not user input
	if err != nil {
		fmt.Fprintf(stderr, "config: không đọc được manifest %s: %v (fail-open)\n", manifestPath, err) //znf:allow-lang
		return 0, nil
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
			fmt.Fprintf(stderr, "config: bỏ %s → %s — %s\n", p.Source, p.Dest, p.Reason) //znf:allow-lang
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
		nWritten, notes := distribute.Apply(plans, readSource, writeDest)
		for _, n := range notes {
			fmt.Fprintln(stdout, "  "+n)
		}
		fmt.Fprintf(stdout, "\nĐã áp dụng: %d ghi (%d giữ nguyên, %d bỏ).\n", nWritten, nSame, nSkip) //znf:allow-lang
		return nWritten, nil
	}

	nChange := 0
	for _, p := range plans {
		if p.State == distribute.Create || p.State == distribute.Update {
			nChange++
		}
	}
	if len(plans) == 0 {
		fmt.Fprintln(stdout, "\nManifest trống — không có cặp nào để phân phối.") //znf:allow-lang
	} else if nChange == 0 {
		fmt.Fprintln(stdout, "\nTất cả đã đồng bộ. (dry-run — dùng --apply để ghi)") //znf:allow-lang
	} else {
		fmt.Fprintf(stdout, "\n%d thay đổi, %d giữ nguyên, %d bỏ. (dry-run — dùng --apply để ghi)\n", nChange, nSame, nSkip) //znf:allow-lang
	}
	return 0, nil
}

// indentBlock indents each line of the diff block by 4 spaces.
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
		Short: "phân phối config workspace-level từ docs/.config (dry-run mặc định)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if workspace == "" {
				workspace, _ = os.Getwd()
			}
			_, err := runConfig(workspace, configDir, apply, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return err
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "thư mục workspace (mặc định cwd)")                                       //znf:allow-lang
	cmd.Flags().StringVar(&configDir, "config-dir", "", "thư mục config nguồn (mặc định repo docs/.config, tự tìm theo layout)") //znf:allow-lang
	cmd.Flags().BoolVar(&apply, "apply", false, "ghi thay đổi (mặc định chỉ dry-run)")                                           //znf:allow-lang
	return cmd
}
