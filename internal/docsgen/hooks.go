package docsgen

import (
	"fmt"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
)

// GenHooks renders hooks.md from the kit's hook table (apply.HookSpecs) plus
// the git-guard hook, which `zenify guard install` wires separately (FR-2.3).
func GenHooks(specs []apply.HookSpec) Files {
	var b strings.Builder
	b.WriteString("---\ntitle: Hook\n---\n\n# Hook\n\n`zenify up` ghi các hook sau vào `~/.claude/settings.json`; mỗi hook là một lệnh `zenify hooks-run <id>` (fail-open, chỉ chạy trong workspace).\n\n") //znf:allow-lang
	b.WriteString("| Event | Matcher | Lệnh | Mục đích |\n|---|---|---|---|\n")                                                                                                                             //znf:allow-lang
	for _, s := range specs {
		m := "—"
		if s.Matcher != "" {
			m = "`" + strings.ReplaceAll(s.Matcher, "|", "\\|") + "`"
		}
		fmt.Fprintf(&b, "| %s | %s | `zenify hooks-run %s` | %s |\n", s.Event, m, s.ID, s.Purpose)
	}
	b.WriteString("| PreToolUse | `Bash` | `zenify git-guard` (cài bằng `zenify guard install`) | Chặn commit/push/merge vào nhánh deploy |\n\n") //znf:allow-lang
	b.WriteString("## Nguồn\n\nSinh bởi `zenify docs gen` từ bảng hook trong binary (`internal/apply/globalhooks.go`).\n") //znf:allow-lang
	return Files{"hooks.md": []byte(b.String())}
}
