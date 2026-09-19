package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/observe"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/readguard"
)

// readGuardPayload is the PreToolUse stdin shape for the Read tool. The
// tool_input field names mirror the Read tool schema the model sends
// (file_path / offset / limit / pages); Claude Code passes tool_input through
// untouched. Offset/Limit are pointers so an absent key is not read as 0.
type readGuardPayload struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath string `json:"file_path"`
		Offset   *int   `json:"offset"`
		Limit    *int   `json:"limit"`
		Pages    string `json:"pages"`
	} `json:"tool_input"`
}

// readDeniedTool is the meter key a denied Read is recorded under, so
// `zenify cost` and the statusline can show how often the guard fired.
const readDeniedTool = "Read:denied"

// runReadGuard is the exit-code core of the read-guard PreToolUse hook,
// shaped like runGitGuard: deny = message on stderr + exit 2, everything else
// (bad JSON, missing file, panic anywhere) = exit 0 and silence. The recovered
// panic value is never printed.
func runReadGuard(stdin io.Reader, stderr io.Writer, stat func(string) (os.FileInfo, error),
	record func(string, string, int64, time.Time) observe.Advice) (code int) {
	defer func() {
		if r := recover(); r != nil {
			code = 0
		}
	}()
	payload, err := io.ReadAll(stdin)
	if err != nil {
		return 0
	}
	var p readGuardPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	if p.ToolName != "Read" || p.ToolInput.FilePath == "" {
		return 0
	}
	d := readguard.Decide(readguard.Input{
		FilePath: p.ToolInput.FilePath,
		Offset:   p.ToolInput.Offset,
		Limit:    p.ToolInput.Limit,
		Pages:    p.ToolInput.Pages,
	}, stat)
	if !d.Deny {
		return 0
	}
	if fi, err := stat(p.ToolInput.FilePath); err == nil {
		_ = record(p.SessionID, readDeniedTool, fi.Size(), observeNow())
	}
	_, _ = fmt.Fprintln(stderr, d.Message)
	return 2
}
