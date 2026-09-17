package cli

import (
	"fmt"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/secretscan"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/spf13/cobra"
)

// newSecretScanCmd builds `zenify secret-scan [path]` — full-tree secret
// scan for CI + manual use. Exits non-zero (via exitcode.Fail) when any
// finding is present; only ever prints Finding.File/RuleID/StartLine/Redacted
// (FR-041 — never the raw secret).
func newSecretScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "secret-scan [path]",
		Short:         "Quét secret trong cây thư mục (dùng cho CI + kiểm tra tay)", //znf:allow-lang
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			s, err := secretscan.New()
			if err != nil {
				return err
			}
			findings, err := s.ScanPath(root)
			if err != nil {
				return err
			}
			u := uiOut(cmd)
			for _, f := range findings {
				u.Step(ui.StatusFail, fmt.Sprintf("%s:%d", f.File, f.StartLine), fmt.Sprintf("%s  %s", f.RuleID, f.Redacted))
			}
			if len(findings) > 0 {
				return exitcode.New(exitcode.Fail, fmt.Errorf("secret-scan: %d finding", len(findings)))
			}
			return nil
		},
	}
}
