package docsgen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// GenCLI renders one page per visible command (cobra/doc) plus cli/index.md.
// Hidden commands get no page; they are listed in the index under
// "Lệnh nội bộ (hook)" so the reader knows they exist (FR-2.1). //znf:allow-lang
func GenCLI(root *cobra.Command) (Files, error) {
	root.DisableAutoGenTag = true
	tmp, err := os.MkdirTemp("", "zenify-docsgen-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	prepender := func(filename string) string {
		base := strings.TrimSuffix(filepath.Base(filename), ".md")
		return fmt.Sprintf("---\ntitle: %s\n---\n\n", strings.ReplaceAll(base, "_", " "))
	}
	linkHandler := func(name string) string {
		return "./" + strings.TrimSuffix(name, ".md")
	}
	if err := doc.GenMarkdownTreeCustom(root, tmp, prepender, linkHandler); err != nil {
		return nil, err
	}
	files := Files{}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(tmp, e.Name()))
		if err != nil {
			return nil, err
		}
		files["cli/"+e.Name()] = b
	}
	files["cli/index.md"] = cliIndex(root)
	return files, nil
}

func cliIndex(root *cobra.Command) []byte {
	var visible, hidden []*cobra.Command
	walkCommands(root, func(c *cobra.Command) {
		if c == root {
			return
		}
		if c.Hidden || anyHiddenAncestor(c) {
			hidden = append(hidden, c)
		} else if c.IsAvailableCommand() {
			visible = append(visible, c)
		}
	})
	byPath := func(s []*cobra.Command) {
		sort.Slice(s, func(i, j int) bool { return s[i].CommandPath() < s[j].CommandPath() })
	}
	byPath(visible)
	byPath(hidden)

	var b strings.Builder
	b.WriteString("---\ntitle: Lệnh CLI\n---\n\n# Lệnh CLI\n\n")                                                    //znf:allow-lang
	b.WriteString("Trang này và mọi trang con được sinh bởi `zenify docs gen` từ chính binary; không sửa tay.\n\n") //znf:allow-lang
	b.WriteString("| Lệnh | Mô tả |\n|---|---|\n")                                                                  //znf:allow-lang
	for _, c := range visible {
		file := strings.ReplaceAll(c.CommandPath(), " ", "_")
		fmt.Fprintf(&b, "| [%s](./%s) | %s |\n", c.CommandPath(), file, c.Short)
	}
	if len(hidden) > 0 {
		b.WriteString("\n## Lệnh nội bộ (hook)\n\nKhông có trang riêng; hook và skill gọi chúng, người dùng không gõ tay.\n\n| Lệnh | Mô tả |\n|---|---|\n") //znf:allow-lang
		for _, c := range hidden {
			fmt.Fprintf(&b, "| `%s` | %s |\n", c.CommandPath(), c.Short)
		}
	}
	return []byte(b.String())
}

func walkCommands(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, sub := range c.Commands() {
		walkCommands(sub, fn)
	}
}

func anyHiddenAncestor(c *cobra.Command) bool {
	for p := c.Parent(); p != nil; p = p.Parent() {
		if p.Hidden {
			return true
		}
	}
	return false
}
