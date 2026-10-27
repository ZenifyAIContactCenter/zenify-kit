package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
)

// HookInput is the subset of the Claude Code hook JSON that hub reads.
type HookInput struct {
	SessionID        string          `json:"session_id"`
	Cwd              string          `json:"cwd"`
	HookEventName    string          `json:"hook_event_name"`
	ToolName         string          `json:"tool_name"`
	ToolInput        json.RawMessage `json:"tool_input"`
	NotificationType string          `json:"notification_type"`
	Prompt           string          `json:"prompt"`
	Source           string          `json:"source"` // SessionStart: startup | resume | clear | compact
}

var slugRe = regexp.MustCompile(`/\.worktrees/([a-z0-9][a-z0-9-]*)(?:[^a-z0-9-]|$)`)

// pathKeys are searched first, in this order, so a path the tool acts on wins
// over a worktree merely mentioned in other input such as file content.
var pathKeys = []string{"file_path", "notebook_path", "path", "command", "cwd"}

// SlugFromToolInput returns the worktree slug named by the first string in the
// tool input that contains a `/.worktrees/<slug>` path, or "" when none does.
func SlugFromToolInput(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	return findSlug(v)
}

func findSlug(v any) string {
	switch x := v.(type) {
	case string:
		if m := slugRe.FindStringSubmatch(x); m != nil {
			return m[1]
		}
	case []any:
		for _, e := range x {
			if s := findSlug(e); s != "" {
				return s
			}
		}
	case map[string]any:
		for _, k := range pathKeys {
			if s := findSlug(x[k]); s != "" {
				return s
			}
		}
		// Remaining keys sorted keep the result deterministic.
		keys := make([]string, 0, len(x))
		for k := range x {
			if !slices.Contains(pathKeys, k) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			if s := findSlug(x[k]); s != "" {
				return s
			}
		}
	}
	return ""
}

// ApplyEvent computes the next agent record for a hook event. remove is true
// when the session ended and its file should be deleted.
func ApplyEvent(prev *Agent, in HookInput, pane string, now int64) (next *Agent, remove bool) {
	if in.HookEventName == "SessionEnd" {
		return nil, true
	}
	var a Agent
	if prev != nil {
		a = *prev
	}
	a.SessionID = in.SessionID
	if pane != "" {
		a.Pane = pane
	}
	a.UpdatedAt = now
	if slug := SlugFromToolInput(in.ToolInput); slug != "" {
		a.Slug = slug
	}
	switch in.HookEventName {
	case "SessionStart":
		// Only compact continues the session in place (FR-4.2); resume,
		// startup, clear and fork start idle.
		if in.Source != "compact" || a.State == "" {
			a.State = Idle
		}
	case "UserPromptSubmit":
		a.State = Working
		if a.Title == "" {
			a.Title = firstRunes(stripControl(in.Prompt), 60)
		}
	case "PreToolUse", "PostToolUse":
		a.State = Working
	case "Notification":
		switch in.NotificationType {
		case "permission_prompt", "agent_needs_input", "elicitation_dialog":
			a.State = Blocked
		default:
			if a.State == "" {
				a.State = Idle
			}
		}
	case "Stop":
		a.State = Done
	default:
		if a.State == "" {
			a.State = Idle
		}
	}
	return &a, false
}

// stripControl maps control runes (newlines, ESC, ...) to spaces so a title
// cannot break or recolour the sidebar row.
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

// maxPayload caps the hook JSON read from stdin.
const maxPayload = 8 << 20

// Report applies one hook event read from stdin to the agent files under home.
// Outside tmux (pane == "") it only drains stdin.
func Report(home string, stdin io.Reader, pane string, now int64) error {
	if pane == "" {
		if _, err := io.Copy(io.Discard, stdin); err != nil {
			return fmt.Errorf("drain hook input: %w", err)
		}
		return nil
	}
	b, err := io.ReadAll(io.LimitReader(stdin, maxPayload+1))
	if err != nil {
		return fmt.Errorf("read hook input: %w", err)
	}
	if len(b) > maxPayload {
		return errors.New("hook payload over 8 MiB")
	}
	var in HookInput
	if err := json.Unmarshal(b, &in); err != nil {
		return fmt.Errorf("decode hook input: %w", err)
	}
	if in.SessionID == "" || strings.ContainsAny(in.SessionID, `/\`) {
		return fmt.Errorf("invalid session_id %q", in.SessionID)
	}
	prev, err := readAgent(home, in.SessionID)
	if err != nil {
		return err
	}
	next, remove := ApplyEvent(prev, in, pane, now)
	if remove {
		if err := os.Remove(agentPath(home, in.SessionID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove agent %s: %w", in.SessionID, err)
		}
		return nil
	}
	return writeAgent(home, next)
}

// LogError appends one line to hub.log. If the log itself cannot be written,
// both errors go to stderr so the failure is never lost.
func LogError(home string, err error, now int64) {
	line := fmt.Sprintf("%s %v\n", time.Unix(now, 0).UTC().Format(time.RFC3339), err)
	werr := os.MkdirAll(Dir(home), 0o700)
	if werr == nil {
		var f *os.File
		f, werr = os.OpenFile(filepath.Join(Dir(home), "hub.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if werr == nil {
			_, werr = f.WriteString(line)
			if cerr := f.Close(); werr == nil {
				werr = cerr
			}
		}
	}
	if werr != nil {
		fmt.Fprintf(os.Stderr, "zenify hub: write hub.log: %v (original error: %v)\n", werr, err)
	}
}
