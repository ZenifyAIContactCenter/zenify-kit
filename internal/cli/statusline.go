package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/observe"
	"github.com/spf13/cobra"
)

// statuslinePayload is the subset of Claude Code's statusLine stdin JSON this
// HUD renders. Every field is optional — Claude Code omits sections that do not
// apply (no cost yet, no context window on the first line), so the renderer must
// treat a zero value as "hide this segment", never as data.
type statuslinePayload struct {
	SessionID string `json:"session_id"`
	Model     struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	ContextWindow struct {
		UsedPercentage float64 `json:"used_percentage"`
	} `json:"context_window"`
	Cost struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
	} `json:"cost"`
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%dKB", n/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// runObserveStatusline is the render core, factored out (like runObserveCount)
// so tests inject the state loaders without touching the filesystem. It ALWAYS
// returns 0 and ALWAYS prints exactly one line (possibly empty) — a statusline
// that errors or prints nothing is worse than one that shows less. The loaders
// supply this kit's own per-session data (dispatch count + tool-output volume),
// which the stdin JSON does not carry; everything else comes from stdin.
func runObserveStatusline(
	stdin io.Reader, stdout io.Writer,
	loadState func(string) (observe.State, bool),
	loadMeter func(string) (observe.Meter, bool),
) (code int) {
	defer func() {
		if r := recover(); r != nil {
			code = 0
		}
	}()

	var p statuslinePayload
	if b, err := io.ReadAll(stdin); err == nil {
		_ = json.Unmarshal(b, &p) // bad/partial JSON → zero value → segments hide
	}

	var seg []string
	if p.Model.DisplayName != "" {
		seg = append(seg, p.Model.DisplayName)
	}
	if p.ContextWindow.UsedPercentage > 0 {
		seg = append(seg, fmt.Sprintf("ctx %.0f%%", p.ContextWindow.UsedPercentage))
	}
	if st, ok := loadState(p.SessionID); ok && st.Count > 0 {
		seg = append(seg, fmt.Sprintf("⟳%d", st.Count))
	}
	if m, ok := loadMeter(p.SessionID); ok {
		if b := m.TotalBytes(); b > 0 {
			seg = append(seg, fmt.Sprintf("↓%s/%d", humanBytes(b), m.TotalCalls()))
		}
	}
	if p.Cost.TotalCostUSD > 0 {
		seg = append(seg, fmt.Sprintf("$%.4f", p.Cost.TotalCostUSD))
	}

	_, _ = fmt.Fprintln(stdout, strings.Join(seg, " · "))
	return 0
}

const statuslineLong = `Render a one-line Claude Code statusline (HUD) from the stdin JSON plus this
kit's per-session observe state (dispatch count from ` + "`zenify observe count`" + ` and
tool-output volume from ` + "`zenify observe meter`" + `).

Wire it up in your settings.json (statusLine is a settings.json key — a plugin
cannot ship one, and only ONE statusline is allowed, so this replaces any
existing one):

  "statusLine": { "type": "command", "command": "zenify observe statusline" }

Segments (each hidden when absent): model · ctx% · ⟳dispatches · ↓tool-output/calls · $cost.`

func newStatuslineCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "statusline",
		Short: "Statusline HUD: model · ctx% · ⟳dispatch · ↓tool-output · $cost",
		Long:  statuslineLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = runObserveStatusline(cmd.InOrStdin(), cmd.OutOrStdout(),
				observe.LoadState, observe.LoadMeter)
			return nil
		},
	}
}
