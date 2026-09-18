// Package skilllint holds content checks for the kit's agent-read markdown
// that are neither language (langgate) nor frontmatter shape (frontmatter).
package skilllint

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Finding is one flagged line.
type Finding struct {
	File string
	Line int
	Text string
}

// SizeBytes and SizeLines cap a SKILL.md (token-diet spec FR-4.1). A skill
// body is injected verbatim into the main context on first invocation and
// stays there for the session, so size is paid on every later turn. Rationale
// belongs in references/<topic>.md, read on demand.
const (
	SizeBytes = 8192
	SizeLines = 200
)

// SizeFinding is one SKILL.md over the cap.
type SizeFinding struct {
	File  string
	Bytes int
	Lines int
}

// ScanSize walks roots and flags every SKILL.md over SizeBytes or SizeLines.
func ScanSize(roots []string) ([]SizeFinding, error) {
	var out []SizeFinding
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || d.Name() != "SKILL.md" {
				return nil
			}
			b, err := os.ReadFile(path) //nolint:gosec // G304 -- path from a trusted scan root
			if err != nil {
				return err
			}
			lines := strings.Count(string(b), "\n")
			if len(b) > SizeBytes || lines > SizeLines {
				out = append(out, SizeFinding{File: path, Bytes: len(b), Lines: lines})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// omitModelRe matches the retired doctrine "omit `model` → inherit the
// session (opus)". Since CLAUDE_CODE_SUBAGENT_MODEL is set by `zenify up`,
// omitting `model` yields sonnet, so any skill that still says otherwise
// sends a dispatch to the wrong tier.
var omitModelRe = regexp.MustCompile("(?i)omit(ting|s|ted)?\\s+`?model`?")

// Scan walks roots and flags every .md line that tells the agent to omit
// `model`. Fenced code blocks are skipped: a snippet may legitimately show a
// call without the key.
func Scan(roots []string) ([]Finding, error) {
	var out []Finding
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			fs, err := scanFile(path)
			if err != nil {
				return err
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
	ln, fenced := 0, false
	for sc.Scan() {
		ln++
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if omitModelRe.MatchString(line) {
			out = append(out, Finding{File: path, Line: ln, Text: strings.TrimSpace(line)})
		}
	}
	return out, sc.Err()
}
