package cli

import (
	"fmt"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/frontmatter"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/langgate"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/skilllint"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/spf13/cobra"
)

func newRulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "quản lý và kiểm rule team (F1/F2/F3)", //znf:allow-lang
	}
	cmd.AddCommand(newRulesLintCmd())
	return cmd
}

func newRulesLintCmd() *cobra.Command {
	var includeGo bool
	cmd := &cobra.Command{
		Use:   "lint [roots...]",
		Short: "kiểm file agent-read: chặn tiếng Việt, frontmatter `globs:` (CC chỉ hiểu `paths:`), doctrine \"omit model\" đã bỏ, và SKILL.md quá 8192 byte / 200 dòng",                                                                                       //znf:allow-lang
		Long:  "Không tham số thì quét asset skill của kit (internal/plugin/assets/znf); thêm --include-go để quét cả internal/**/*.go (đã dịch xong, gate bật). Truyền path cụ thể để quét nơi khác, vd: zenify rules lint ~/.zenify/knowledge/.config/rules", //znf:allow-lang
		RunE: func(cmd *cobra.Command, args []string) error {
			roots := args
			if len(roots) == 0 {
				roots = []string{"internal/plugin/assets/znf"}
			}
			vs, err := langgate.Scan(roots, includeGo)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			fms, err := frontmatter.Scan(roots)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			sls, err := skilllint.Scan(roots)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			szs, err := skilllint.ScanSize(roots)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			u := uiOut(cmd)
			if len(vs) == 0 && len(fms) == 0 && len(sls) == 0 && len(szs) == 0 {
				u.Step(ui.StatusOK, "rules lint: sạch.", "") //znf:allow-lang
				return nil
			}
			for _, v := range vs {
				u.Step(ui.StatusFail, fmt.Sprintf("%s:%d: %s", v.File, v.Line, v.Text), "")
			}
			for _, f := range fms {
				u.Step(ui.StatusFail, fmt.Sprintf("%s:%d: %s", f.File, f.Line, f.Text), "")
			}
			for _, f := range sls {
				u.Step(ui.StatusFail, fmt.Sprintf("%s:%d: omit-model doctrine retired (CLAUDE_CODE_SUBAGENT_MODEL) — name the tier: %s", f.File, f.Line, f.Text), "")
			}
			for _, f := range szs {
				u.Step(ui.StatusFail, fmt.Sprintf("%s: %d bytes / %d lines; cap %d bytes / %d lines — move rationale to references/", f.File, f.Bytes, f.Lines, skilllint.SizeBytes, skilllint.SizeLines), "")
			}
			return exitcode.New(exitcode.Fail,
				fmt.Errorf("rules lint: %d dòng tiếng Việt, %d frontmatter globs:, %d dòng \"omit model\", %d SKILL.md quá cỡ", len(vs), len(fms), len(sls), len(szs))) //znf:allow-lang
		},
	}
	cmd.Flags().BoolVar(&includeGo, "include-go", false, "quét cả internal/**/*.go") //znf:allow-lang
	return cmd
}
