package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// tabTitleWindow is how much of a transcript's end is searched for a title.
const tabTitleWindow = 1 << 20

// readTabTitle returns the Claude Code tab name of a session: the newest custom-title line of its
// transcript (~/.claude/projects/*/<session>.jsonl), else the newest ai-title line. A missing
// transcript or title gives "" and no error.
func readTabTitle(session string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	// The session id comes from a file name under live/; a separator or glob char must not widen the glob.
	if session == "" || strings.ContainsAny(session, "/\\*?[") {
		return "", nil
	}
	path, err := findTranscript(filepath.Join(home, ".claude", "projects"), session)
	if err != nil || path == "" {
		return "", err
	}
	f, err := os.Open(path) //nolint:gosec // G304 -- listed under ~/.claude/projects
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	start := int64(0)
	if fi.Size() > tabTitleWindow {
		start = fi.Size() - tabTitleWindow
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	buf, err := io.ReadAll(io.LimitReader(f, tabTitleWindow))
	if err != nil {
		return "", err
	}
	lines := bytes.Split(buf, []byte("\n"))
	if start > 0 && !json.Valid(lines[0]) {
		lines = lines[1:] // the window starts mid-line
	}
	ai := ""
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if !bytes.Contains(l, []byte(`"custom-title"`)) && !bytes.Contains(l, []byte(`"ai-title"`)) {
			continue
		}
		var d struct {
			Type        string `json:"type"`
			CustomTitle string `json:"customTitle"`
			AITitle     string `json:"aiTitle"`
		}
		if json.Unmarshal(l, &d) != nil {
			continue
		}
		switch d.Type {
		case "custom-title":
			if t := cleanTitle(d.CustomTitle); t != "" {
				return t, nil
			}
		case "ai-title":
			if t := cleanTitle(d.AITitle); t != "" && ai == "" {
				ai = t
			}
		}
	}
	return ai, nil
}

// findTranscript returns <projects>/*/<session>.jsonl, or "" when none exists. Unlike filepath.Glob
// it surfaces read errors, so an unreadable project dir is reported rather than read as "no title".
func findTranscript(projects, session string) (string, error) {
	dirs, err := os.ReadDir(projects)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	for _, d := range dirs {
		if !d.IsDir() && d.Type()&os.ModeSymlink == 0 {
			continue
		}
		p := filepath.Join(projects, d.Name(), session+".jsonl")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "", nil
}

// cleanTitle drops control characters and trims the spaces.
func cleanTitle(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
}
