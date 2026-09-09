package cli

import (
	"fmt"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/langgate"
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
		Short: "chặn tiếng Việt trong file agent-read (skill .md, Go, rules)",                                                                                                                                                                                  //znf:allow-lang
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
			if len(vs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "rules lint: sạch.") //znf:allow-lang
				return nil
			}
			for _, v := range vs {
				fmt.Fprintf(cmd.OutOrStdout(), "%s:%d: %s\n", v.File, v.Line, v.Text)
			}
			return exitcode.New(exitcode.Fail,
				fmt.Errorf("rules lint: %d dòng tiếng Việt trong file agent-read", len(vs))) //znf:allow-lang
		},
	}
	cmd.Flags().BoolVar(&includeGo, "include-go", false, "quét cả internal/**/*.go") //znf:allow-lang
	return cmd
}
