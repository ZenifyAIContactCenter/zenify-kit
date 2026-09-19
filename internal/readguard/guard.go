// Package readguard decides whether a Read tool call would pull a result into
// the main context that is too large to be there by accident. It is a pure
// rule: input shape + a stat function in, allow/deny + a hint out. Every error
// path falls open (allow) — the guard exists to stop accidents, not to be one.
package readguard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Thresholds. Measured 2026-09-19 over 30 days of one workspace: 23 tool
// results above 100 KB, every one a Read of a whole PDF or a screenshot, each
// resident in context until /clear. Text is the file's size on disk, not the
// slice read; images are bytes, not pixels.
const (
	TextLimitBytes  = 200_000
	ImageLimitBytes = 300_000
)

// Input mirrors the fields of the Read tool's tool_input as the model sends
// them. Offset and Limit are pointers so "absent" and "0" stay distinct.
type Input struct {
	FilePath string
	Offset   *int
	Limit    *int
	Pages    string
}

// Decision is what the hook turns into exit 2 + stderr (Deny) or exit 0.
type Decision struct {
	Deny    bool
	Message string
}

var imageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}

// Decide applies the three rules. stat is injected so tests need no real
// files beyond t.TempDir() and so a panic inside os.Stat is the caller's to
// recover.
func Decide(in Input, stat func(string) (os.FileInfo, error)) Decision {
	if in.FilePath == "" {
		return Decision{}
	}
	fi, err := stat(in.FilePath)
	if err != nil || !fi.Mode().IsRegular() {
		return Decision{} // missing, unreadable, directory → fall open
	}
	size := fi.Size()
	kb := size / 1000
	ext := strings.ToLower(filepath.Ext(in.FilePath))
	switch {
	case ext == ".pdf" && in.Pages == "":
		return Decision{Deny: true, Message: fmt.Sprintf(
			"znf read-guard: Read of %s (%d KB, PDF) without `pages` would load the whole document into context. "+
				"Re-run with `pages` (e.g. \"1-5\"), or dispatch a subagent to summarise it.", in.FilePath, kb)}
	case imageExt[ext] && size > ImageLimitBytes:
		return Decision{Deny: true, Message: fmt.Sprintf(
			"znf read-guard: image %s is %d KB (limit %d KB) and would stay in context for the rest of the session. "+
				"Resize it first, or let znf:ui-verifier look at it and return a verdict.", in.FilePath, kb, ImageLimitBytes/1000)}
	case !imageExt[ext] && size > TextLimitBytes && (in.Offset == nil || in.Limit == nil):
		return Decision{Deny: true, Message: fmt.Sprintf(
			"znf read-guard: %s is %d KB (limit %d KB). Read it in slices with both `offset` and `limit`, "+
				"grep for the part you need, or dispatch a subagent to summarise it.", in.FilePath, kb, TextLimitBytes/1000)}
	}
	return Decision{}
}
