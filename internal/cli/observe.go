package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/observe"
	"github.com/spf13/cobra"
)

type observePayload struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
}

// observeMeterPayload is the PostToolUse stdin shape. tool_response is captured
// raw so its serialized byte length is the metered volume, whatever its type
// (string for Bash, object for structured tools).
type observeMeterPayload struct {
	SessionID    string          `json:"session_id"`
	ToolName     string          `json:"tool_name"`
	ToolResponse json.RawMessage `json:"tool_response"`
}

var observeNow = time.Now // seam for tests

// runObserveCount is the exit-code core, factored out (like runGitGuard) so
// tests inject bump without exec'ing the binary. It ALWAYS returns 0 — a soft
// cap never blocks — and a deferred recover keeps even a panic at exit 0.
func runObserveCount(stdin io.Reader, stdout io.Writer, getenv func(string) string,
	bump func(string, int, time.Time) observe.Decision) (code int) {
	defer func() {
		if r := recover(); r != nil {
			code = 0
		}
	}()
	payload, err := io.ReadAll(stdin)
	if err != nil {
		return 0
	}
	var p observePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	if p.ToolName != "Task" {
		return 0
	}
	softCap := observe.ResolveCap(getenv)
	if softCap <= 0 {
		return 0 // disabled
	}
	dec := bump(p.SessionID, softCap, observeNow())
	if !dec.Warn {
		return 0
	}
	out := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "PreToolUse",
			"additionalContext": dec.Message,
		},
	}
	b, err := json.Marshal(out)
	if err != nil {
		return 0
	}
	_, _ = fmt.Fprintln(stdout, string(b))
	return 0
}

// runObserveMeter is the exit-code core for the PostToolUse meter hook, factored
// out (like runObserveCount) so tests inject record without exec'ing the binary.
// It ALWAYS returns 0 — metering is passive and never blocks — and a deferred
// recover keeps even a panic at exit 0. Tool scoping is done by the hooks.json
// matcher, not here, so the matcher can change without a recompile.
func runObserveMeter(stdin io.Reader, record func(string, string, int64, time.Time)) (code int) {
	defer func() {
		if r := recover(); r != nil {
			code = 0
		}
	}()
	payload, err := io.ReadAll(stdin)
	if err != nil {
		return 0
	}
	var p observeMeterPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	if p.ToolName == "" {
		return 0
	}
	record(p.SessionID, p.ToolName, int64(len(p.ToolResponse)), observeNow())
	return 0
}

func newObserveCmd() *cobra.Command {
	c := &cobra.Command{Use: "observe", Short: "Observability: đếm/nhắc fan-out subagent"}
	count := &cobra.Command{
		Use:    "count",
		Short:  "PreToolUse hook: đếm dispatch Task + soft-cap warn (không chặn)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = runObserveCount(cmd.InOrStdin(), cmd.OutOrStdout(), os.Getenv, observe.Bump)
			return nil // always allow (exit 0)
		},
	}
	meter := &cobra.Command{
		Use:    "meter",
		Short:  "PostToolUse hook: đo lượng tool-output per-session (passive, không sửa output)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = runObserveMeter(cmd.InOrStdin(), observe.Record)
			return nil // always allow (exit 0)
		},
	}
	c.AddCommand(count)
	c.AddCommand(meter)
	c.AddCommand(newStatuslineCmd())
	return c
}
