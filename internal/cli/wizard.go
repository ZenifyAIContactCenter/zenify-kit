package cli

import (
	"io"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

// runWizard is a Task 8 stub — it rebuilds the plan and prints it as a table,
// same as the headless dry-run path. Tasks 9-11 replace this body with the
// real interactive TUI.
func runWizard(w io.Writer, m *manifest.Manifest, workspace string) error {
	plans, auth, err := buildPlan(m, ghx.ExecRunner(), gitx.ExecRunner(), workspace)
	if err != nil {
		return err
	}
	renderPlanTable(w, plans, auth)
	return nil
}
