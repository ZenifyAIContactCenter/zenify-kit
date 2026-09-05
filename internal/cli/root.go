package cli

import (
	"sync"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
	"github.com/spf13/cobra"
)

var defaultChecksOnce sync.Once

// NewRootCmd builds the top-level `zenify` command. Subcommands attach here.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "zenify",
		Short:         "zenify — portable workspace toolkit",
		Version:       version.Current(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// A flag-parse error is a usage error → exit 2 (BadArgs), not 1.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return exitcode.New(exitcode.BadArgs, err)
	})
	root.AddCommand(newVersionCmd())
	defaultChecksOnce.Do(registerDefaultChecks)
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newUpCmd())
	root.AddCommand(newWtCmd())
	root.AddCommand(newDBReadCmd())
	root.AddCommand(newGitGuardCmd())
	root.AddCommand(newSecretScanCmd())
	root.AddCommand(newGuardCmd())
	root.AddCommand(newSkillsCmd())
	root.AddCommand(newGateCmd())
	root.AddCommand(newHotfixCmd())
	root.AddCommand(newObserveCmd())
	root.AddCommand(newReviewVerifyCmd())
	root.AddCommand(newReviewBundleCmd())
	root.AddCommand(newReviewDoctrineCmd())
	root.AddCommand(newReviewAdviseGateCmd())
	root.AddCommand(newReviewLogCmd())
	root.AddCommand(newAnalyzeCmd())
	root.AddCommand(newStandardsCmd())
	root.AddCommand(newReleaseReportCmd())
	root.AddCommand(newConfigCmd())
	return root
}
