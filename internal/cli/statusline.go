package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/observe"
	"github.com/spf13/cobra"
)

// statuslineCommand is the settings.json statusLine command that `install` writes.
const statuslineCommand = "zenify observe statusline"

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
//
// segmentOnly renders JUST the two kit-unique segments (⟳dispatches · ↓tool-out)
// with no leading/trailing separator, for splicing into a statusline the user
// already owns — the model/ctx/cost segments are dropped because that existing
// line already shows them. Full mode renders every segment as a standalone line.
func runObserveStatusline(
	stdin io.Reader, stdout io.Writer, segmentOnly bool,
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
	if !segmentOnly && p.Model.DisplayName != "" {
		seg = append(seg, p.Model.DisplayName)
	}
	if !segmentOnly && p.ContextWindow.UsedPercentage > 0 {
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
	if !segmentOnly && p.Cost.TotalCostUSD > 0 {
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

Segments (each hidden when absent): model · ctx% · ⟳dispatches · ↓tool-output/calls · $cost.

--segment renders ONLY this kit's own two segments (⟳dispatches · ↓tool-output)
and drops model/ctx/cost. Use it when you already have a statusline you like:
keep your script, pipe its same stdin JSON to this, and append the output — e.g.

  seg=$(printf '%s' "$input" | zenify observe statusline --segment)
  [ -n "$seg" ] && line2+="  |  $seg"`

// ensureStatusline merges the kit's statusLine command into settings.json JSON.
// It NEVER clobbers a statusline the user already set unless force is true — the
// single statusLine slot is theirs, so an occupied slot returns action "occupied"
// with the input unchanged, and the caller points them at --segment instead.
// action ∈ {installed, already, occupied, forced}; out is meaningful only for
// installed/forced (the cases the caller writes). Broken JSON is an error.
func ensureStatusline(raw []byte, force bool) (out []byte, action string, err error) {
	root := map[string]any{}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &root); err != nil {
			return nil, "", fmt.Errorf("statusline install: settings.json không parse được: %w", err)
		}
	}

	desired := map[string]any{"type": "command", "command": statuslineCommand}

	if existing, ok := root["statusLine"].(map[string]any); ok {
		if cmd, _ := existing["command"].(string); cmd == statuslineCommand {
			return nil, "already", nil
		}
		if !force {
			return nil, "occupied", nil
		}
		action = "forced"
	} else {
		action = "installed"
	}

	root["statusLine"] = desired
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, "", err
	}
	return append(b, '\n'), action, nil
}

func newStatuslineInstallCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "install",
		Short: "Ghi key statusLine → `zenify observe statusline` vào ~/.claude/settings.json (chỉ khi trống)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("statusline install: không xác định được HOME: %w", err)
			}
			path := filepath.Join(home, ".claude", "settings.json")
			raw, err := os.ReadFile(path) //nolint:gosec // G304 -- fixed config location under the user's own HOME
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("statusline install: đọc %s: %w", path, err)
			}
			out, action, err := ensureStatusline(raw, force)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			switch action {
			case "already":
				_, _ = fmt.Fprintln(w, "statusline install: đã cấu hình sẵn (idempotent).")
				return nil
			case "occupied":
				_, _ = fmt.Fprintln(w, "statusline install: settings.json đã có statusLine khác — KHÔNG đè.")
				_, _ = fmt.Fprintln(w, "  Giữ statusline của bạn và chèn segment: `zenify observe statusline --segment` (xem `--help`).")
				_, _ = fmt.Fprintln(w, "  Hoặc đè hẳn bằng: `zenify observe statusline install --force`.")
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				return fmt.Errorf("statusline install: tạo thư mục %s: %w", filepath.Dir(path), err)
			}
			perm := os.FileMode(0o644)
			if fi, statErr := os.Stat(path); statErr == nil {
				perm = fi.Mode().Perm()
			}
			if err := writeFileAtomic(path, out, perm); err != nil {
				return fmt.Errorf("statusline install: ghi %s: %w", path, err)
			}
			if action == "forced" {
				_, _ = fmt.Fprintln(w, "statusline install: đã ĐÈ statusLine → zenify observe statusline (--force).")
			} else {
				_, _ = fmt.Fprintln(w, "statusline install: đã đặt statusLine → zenify observe statusline.")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&force, "force", false, "đè statusLine hiện có (mặc định từ chối nếu đã có cái khác)")
	return c
}

func newStatuslineCmd() *cobra.Command {
	var segmentOnly bool
	c := &cobra.Command{
		Use:   "statusline",
		Short: "Statusline HUD: model · ctx% · ⟳dispatch · ↓tool-output · $cost",
		Long:  statuslineLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = runObserveStatusline(cmd.InOrStdin(), cmd.OutOrStdout(), segmentOnly,
				observe.LoadState, observe.LoadMeter)
			return nil
		},
	}
	c.Flags().BoolVar(&segmentOnly, "segment", false,
		"render only ⟳dispatch · ↓tool-output, for splicing into an existing statusline")
	c.AddCommand(newStatuslineInstallCmd())
	return c
}
