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
