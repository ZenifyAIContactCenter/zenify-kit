// Package distribute distributes workspace-level config files from docs/.config/
// down into the workspace via a manifest. Source = config/, one-way. Fail-open.
package distribute

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

type State string

const (
	Create State = "CREATE"
	Same   State = "SAME"
	Update State = "UPDATE"
	Skip   State = "SKIP"
)

// Pair is one manifest line: source file (in the config dir) → dest (in the workspace).
type Pair struct{ Source, Dest string }

// FilePlan is the classification for one Pair after comparing content.
type FilePlan struct {
	Source, Dest string
	State        State
	Diff         string // unified diff for UPDATE; "" otherwise
	Reason       string // reason when SKIP; "" otherwise
}

// escapesRoot reports an empty path, an absolute path, or one that escapes the root dir via "..".
func escapesRoot(p string) bool {
	if p == "" || filepath.IsAbs(p) {
		return true
	}
	c := filepath.Clean(p)
	return c == ".." || strings.HasPrefix(c, ".."+string(filepath.Separator))
}

// ParseManifest reads "<source> <dest>" lines. Skips #-prefixed and blank lines.
// A line without exactly 2 fields (missing dest, or extra fields) → adds a note, skips it (not an error).
func ParseManifest(b []byte) ([]Pair, []string) {
	var pairs []Pair
	var notes []string
	for _, ln := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(ln)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		f := strings.Fields(s)
		if len(f) != 2 {
			notes = append(notes, "skipping manifest line without exactly 2 fields: "+s)
			continue
		}
		pairs = append(pairs, Pair{Source: f[0], Dest: f[1]})
	}
	return pairs, notes
}

// Plan classifies each Pair by comparing source vs dest content. Pure: all I/O
// is injected for testing. config/ is the source → diff runs dest→source.
func Plan(pairs []Pair, readSource, readDest func(string) ([]byte, error), isNotFound func(error) bool) []FilePlan {
	out := make([]FilePlan, 0, len(pairs))
	for _, p := range pairs {
		fp := FilePlan{Source: p.Source, Dest: p.Dest}
		if escapesRoot(p.Source) || escapesRoot(p.Dest) {
			fp.State = Skip
			fp.Reason = "path escapes root (absolute or ..)"
			out = append(out, fp)
			continue
		}
		srcB, err := readSource(p.Source)
		if err != nil {
			fp.State = Skip
			fp.Reason = "could not read source"
			out = append(out, fp)
			continue
		}
		dstB, err := readDest(p.Dest)
		if err != nil {
			if isNotFound(err) {
				fp.State = Create
			} else {
				fp.State = Skip
				fp.Reason = "could not read dest"
			}
			out = append(out, fp)
			continue
		}
		if bytes.Equal(srcB, dstB) {
			fp.State = Same
			out = append(out, fp)
			continue
		}
		fp.State = Update
		fp.Diff = unifiedDiff(dstB, srcB, p.Dest, p.Source)
		out = append(out, fp)
	}
	return out
}

func unifiedDiff(dst, src []byte, fromName, toName string) string {
	text, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(dst)),
		B:        difflib.SplitLines(string(src)),
		FromFile: fromName,
		ToFile:   toName,
		Context:  3,
	})
	return strings.TrimRight(text, "\n")
}

// ExpandDirPairs replaces each directory pair (Source ending in "/") with one
// file pair per entry that listDir returns; file pairs pass through unchanged.
// Fail-open: a listDir error drops that dir pair with a note, no error returned.
func ExpandDirPairs(pairs []Pair, listDir func(string) ([]string, error)) ([]Pair, []string) {
	var out []Pair
	var notes []string
	for _, p := range pairs {
		if !strings.HasSuffix(p.Source, "/") {
			out = append(out, p)
			continue
		}
		entries, err := listDir(p.Source)
		if err != nil {
			notes = append(notes, "bỏ dir-pair "+p.Source+": "+err.Error()) //znf:allow-lang
			continue
		}
		for _, e := range entries {
			out = append(out, Pair{Source: p.Source + e, Dest: p.Dest + e})
		}
	}
	return out, notes
}

// Apply writes CREATE/UPDATE files (re-reads source via readSource, writes via writeDest).
// SAME/SKIP write nothing. Fail-open: read/write errors become a note, do not stop.
// Returns the number of files written successfully (typed count, so callers need not
// parse the human-readable notes) alongside those notes.
func Apply(plans []FilePlan, readSource func(string) ([]byte, error), writeDest func(dest string, data []byte) error) (int, []string) {
	written := 0
	var notes []string
	for _, p := range plans {
		if p.State != Create && p.State != Update {
			continue
		}
		b, err := readSource(p.Source)
		if err != nil {
			notes = append(notes, "could not read source "+p.Source+": "+err.Error())
			continue
		}
		if err := writeDest(p.Dest, b); err != nil {
			notes = append(notes, "could not write "+p.Dest+": "+err.Error())
			continue
		}
		written++
		notes = append(notes, "đã ghi "+p.Dest) //znf:allow-lang
	}
	return written, notes
}
