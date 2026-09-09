// Package langgate flags non-English (Vietnamese) prose in agent-read files.
// A file is "agent-read" if it is a skill/rule .md or Go source; its comments,
// identifiers and prose must be English. User-facing CLI output strings opt out
// per line with an allow-lang marker. Pure: reads files, returns violations.
package langgate

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Violation is one flagged line.
type Violation struct {
	File string
	Line int
	Text string
}

const (
	goMarker = "//znf:allow-lang"
	mdMarker = "<!-- znf:allow-lang -->"
)

// Scan walks each root and flags lines containing a Vietnamese diacritic in an
// agent-read position. .md files are always scanned; .go files only when
// includeGo is true.
func Scan(roots []string, includeGo bool) ([]Violation, error) {
	var out []Violation
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".md":
				vs, e := scanMarkdown(path)
				if e != nil {
					return e
				}
				out = append(out, vs...)
			case ".go":
				if includeGo {
					vs, e := scanGo(path)
					if e != nil {
						return e
					}
					out = append(out, vs...)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// hasNonASCIILetter reports whether s contains a non-ASCII letter rune —
// accented Latin and Vietnamese diacritics are letters, so this doubles as an
// "English/ASCII-only" check; em dash, arrow and middle dot are punctuation, not
// letters, so they pass.
func hasNonASCIILetter(s string) bool {
	for _, r := range s {
		if r > 127 && unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func scanGo(path string) ([]Violation, error) {
	return scanLines(path, func(line string) bool {
		if strings.Contains(line, goMarker) {
			return false
		}
		return hasNonASCIILetter(line)
	})
}

func scanMarkdown(path string) ([]Violation, error) {
	f, err := os.Open(path) //nolint:gosec // G304 -- path from a trusted scan root
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Violation
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inFence := false
	ln := 0
	for sc.Scan() {
		ln++
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.Contains(line, mdMarker) {
			continue
		}
		if hasNonASCIILetter(stripInlineCode(line)) {
			out = append(out, Violation{File: path, Line: ln, Text: line})
		}
	}
	return out, sc.Err()
}

// stripInlineCode removes `...` spans so a Vietnamese literal quoted as inline
// code (a data string) is not flagged.
func stripInlineCode(s string) string {
	var b strings.Builder
	inCode := false
	for _, r := range s {
		if r == '`' {
			inCode = !inCode
			continue
		}
		if !inCode {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func scanLines(path string, flag func(string) bool) ([]Violation, error) {
	f, err := os.Open(path) //nolint:gosec // G304 -- path from a trusted scan root
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Violation
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	ln := 0
	for sc.Scan() {
		ln++
		line := sc.Text()
		if flag(line) {
			out = append(out, Violation{File: path, Line: ln, Text: line})
		}
	}
	return out, sc.Err()
}
