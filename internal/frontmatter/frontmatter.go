// Package frontmatter flags a rule/skill .md whose YAML frontmatter scopes with
// a key Claude Code does not recognise. Claude Code path-scopes on the `paths:`
// key (a YAML list); a `globs:` key (the Cursor spelling) is silently ignored,
// so the rule loads unconditionally every session instead of only when a
// matching file is read. Pure: reads files, returns findings.
package frontmatter

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Finding is one flagged frontmatter line.
type Finding struct {
	File string
	Line int
	Text string
}

// Message is the fixed advice emitted for every flagged line.
const Message = "frontmatter key `globs:` is ignored by Claude Code — use `paths:` (a YAML list)"

// Scan walks each root and flags every .md file whose frontmatter declares a
// top-level `globs:` key. Only the leading frontmatter block is inspected, so a
// `globs:` mentioned in the body is not flagged.
func Scan(roots []string) ([]Finding, error) {
	var out []Finding
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if strings.ToLower(filepath.Ext(path)) != ".md" {
				return nil
			}
			fs, e := scanFile(path)
			if e != nil {
				return e
			}
			out = append(out, fs...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanFile(path string) ([]Finding, error) {
	f, err := os.Open(path) //nolint:gosec // G304 -- path from a trusted scan root
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []Finding
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	ln := 0
	opened := false // seen the opening --- fence
	for sc.Scan() {
		ln++
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if !opened {
			if trimmed == "" {
				continue // tolerate blank lines before the frontmatter
			}
			if trimmed != "---" {
				return out, nil // first content is not a fence: no frontmatter
			}
			opened = true
			continue
		}
		if trimmed == "---" {
			break // end of frontmatter
		}
		// A YAML top-level key sits at column 0; an indented line is nested.
		if strings.HasPrefix(line, "globs:") {
			out = append(out, Finding{File: path, Line: ln, Text: Message})
		}
	}
	return out, sc.Err()
}
