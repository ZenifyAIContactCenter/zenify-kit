// Package docsweep plans and applies the knowledge store's lifecycle cleanup: plans of built or
// superseded specs, stale handoffs and unlinked references are deleted (after a local demotion
// copy), and spec status headers are written from speclife state. Pure apart from the FS seam:
// no git, no network.
package docsweep

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/speclife"
)

// FS is every filesystem operation docsweep needs, behind a seam so tests run in memory.
// Rename backs the temp+rename header write.
type FS interface {
	ReadDir(string) ([]os.DirEntry, error)
	ReadFile(string) ([]byte, error)
	WriteFile(string, []byte, os.FileMode) error
	MkdirAll(string, os.FileMode) error
	Remove(string) error
	Rename(oldpath, newpath string) error
	Stat(string) (os.FileInfo, error)
}

// OSFS implements FS with the os package.
type OSFS struct{}

func (OSFS) ReadDir(p string) ([]os.DirEntry, error)           { return os.ReadDir(p) }
func (OSFS) ReadFile(p string) ([]byte, error)                 { return os.ReadFile(p) } //nolint:gosec // G304 -- OSFS is the real-filesystem seam, paths come from walking the store
func (OSFS) WriteFile(p string, b []byte, m os.FileMode) error { return os.WriteFile(p, b, m) }
func (OSFS) MkdirAll(p string, m os.FileMode) error            { return os.MkdirAll(p, m) }
func (OSFS) Remove(p string) error                             { return os.Remove(p) }
func (OSFS) Rename(oldpath, newpath string) error              { return os.Rename(oldpath, newpath) }
func (OSFS) Stat(p string) (os.FileInfo, error)                { return os.Stat(p) }

// Action is one planned change. Path is relative to the store, "/"-separated.
type Action struct {
	Kind, Path, Detail string
}

const (
	KindDeletePlan      = "delete-plan"
	KindDeleteHandoff   = "delete-handoff"
	KindDeleteReference = "delete-reference"
	KindHeader          = "header"
)

// kindOrder sorts actions by the order of the Kind constants above.
var kindOrder = map[string]int{KindDeletePlan: 0, KindDeleteHandoff: 1, KindDeleteReference: 2, KindHeader: 3}

// handoffMaxAge: a dated handoff older than this is deleted.
const handoffMaxAge = 30 * 24 * time.Hour

var datePrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

var (
	statusLine = regexp.MustCompile(`^\*\*(Trạng thái|Status):\*\*\s*(.*)$`) //znf:allow-lang
	labelLine  = regexp.MustCompile(`^\*\*[^*]+:\*\*`)
)

// IsHeaderRewrite reports whether after is exactly what the sweep's header write makes of before:
// the status header set to after's first status value, a state the sweep writes, and nothing
// else changed. Only the header block (before the first "---" or "## ") counts. docsync uses it to
// recognise a machine-only commit; a status-like line edited in the body, or a hand-written value,
// does not qualify.
func IsHeaderRewrite(before, after []byte) bool {
	for _, l := range strings.Split(string(after), "\n") {
		if l == "---" || strings.HasPrefix(l, "## ") {
			return false // past the header block: a status-like line here is body content
		}
		if m := statusLine.FindStringSubmatch(l); m != nil {
			switch st := speclife.State(strings.TrimSpace(m[2])); st {
			case speclife.StatePlanned, speclife.StateInProgress, speclife.StateBuilt, speclife.StateBuiltFuzzy, speclife.StateSuperseded:
				out, changed, _ := rewriteHeader(before, st)
				return changed && string(out) == string(after)
			}
			return false // a value the sweep never writes is a person's edit
		}
	}
	return false
}

// detailArrow separates the old and new value in a header action's Detail; Apply reads the new
// state back from the text after it.
const detailArrow = " → "

// statusLabel is the label rewriteHeader inserts when a spec has no status line.
const statusLabel = "**Trạng thái:** " //znf:allow-lang

// Plan computes the sweep's actions from the store contents and the spec statuses. It never
// writes. Notes report per-directory read failures; a missing directory is simply empty.
func Plan(fs FS, store string, statuses []speclife.Status, now time.Time) ([]Action, []string) {
	var acts []Action
	var notes []string
	abs := func(rel string) string { return filepath.Join(store, filepath.FromSlash(rel)) }
	walk := func(dir string) []string {
		files, err := walkFiles(fs, abs(dir))
		if err != nil {
			notes = append(notes, "docs sweep: read "+dir+": "+err.Error())
		}
		out := make([]string, len(files))
		for i, f := range files {
			out[i] = path.Join(dir, f)
		}
		return out
	}

	// Plans of built or superseded specs.
	deletedPlans := map[string]bool{}
	for _, s := range statuses {
		if !s.HasPlan || (s.State != speclife.StateBuilt && s.State != speclife.StateSuperseded) {
			continue
		}
		p := path.Join("plans", speclife.RepoOf(s.Path), planStem(s.Path)+".md")
		if _, err := fs.Stat(abs(p)); err != nil {
			continue
		}
		deletedPlans[p] = true
		acts = append(acts, Action{Kind: KindDeletePlan, Path: p, Detail: "spec " + string(s.State)})
	}

	// Handoffs: undated, or dated older than handoffMaxAge.
	cutoff := now.Add(-handoffMaxAge)
	for _, p := range walk("handoff") {
		date, ok := handoffDate(strings.TrimPrefix(p, "handoff/"))
		switch {
		case !ok:
			acts = append(acts, Action{Kind: KindDeleteHandoff, Path: p, Detail: "undated"})
		case date.Before(cutoff):
			acts = append(acts, Action{Kind: KindDeleteHandoff, Path: p, Detail: "dated " + date.Format("2006-01-02")})
		}
	}

	// References no spec, surviving plan or .config file links by full relative path.
	// A source that could not be read might be the one linking a reference, so any read failure
	// skips the rule rather than deleting on incomplete evidence.
	var sources []string
	sourceErr := false
	for _, dir := range []string{"specs", "plans", ".config"} {
		files, err := walkFiles(fs, abs(dir))
		if err != nil {
			sourceErr = true
			notes = append(notes, "docs sweep: read "+dir+": "+err.Error())
		}
		for _, f := range files {
			p := path.Join(dir, f)
			if deletedPlans[p] {
				continue
			}
			if b, err := fs.ReadFile(abs(p)); err == nil {
				sources = append(sources, string(b))
			} else {
				sourceErr = true
				notes = append(notes, "docs sweep: read "+p+": "+err.Error())
			}
		}
	}
	if sourceErr {
		notes = append(notes, "docs sweep: skip reference rule: a link source could not be read")
	} else {
		for _, p := range walk("reference") {
			if !linked(sources, p) {
				acts = append(acts, Action{Kind: KindDeleteReference, Path: p, Detail: "unlinked"})
			}
		}
	}

	// Status headers of specs whose state is known.
	for _, s := range statuses {
		if !strings.HasSuffix(s.Path, "-design.md") || s.State == speclife.StateUnknown {
			continue
		}
		b, err := fs.ReadFile(abs(s.Path))
		if err != nil {
			notes = append(notes, "docs sweep: read "+s.Path+": "+err.Error())
			continue
		}
		if _, changed, note := rewriteHeader(b, s.State); changed {
			acts = append(acts, Action{Kind: KindHeader, Path: s.Path, Detail: note})
		} else if note != "" {
			notes = append(notes, "docs sweep: skip header "+s.Path+" "+note)
		}
	}

	sort.SliceStable(acts, func(i, j int) bool {
		if acts[i].Kind != acts[j].Kind {
			return kindOrder[acts[i].Kind] < kindOrder[acts[j].Kind]
		}
		return acts[i].Path < acts[j].Path
	})
	return acts, notes
}

func linked(sources []string, p string) bool {
	for _, src := range sources {
		if strings.Contains(src, p) {
			return true
		}
	}
	return false
}

// Apply performs actions. Every deletion first copies the file to a local demotion dir
// (reference → <viewDir>/research, handoff → <viewDir>/notes, plans → <viewDir>/notes/plans);
// a failed copy keeps the file. Headers are written via temp + rename, keeping the file mode.
// Notes report failures only; re-running Plan after Apply yields nothing new.
func Apply(fs FS, store, viewDir string, actions []Action) []string {
	var notes []string
	for _, a := range actions {
		src := filepath.Join(store, filepath.FromSlash(a.Path))
		if a.Kind == KindHeader {
			i := strings.LastIndex(a.Detail, detailArrow)
			if i < 0 {
				notes = append(notes, "docs sweep: header "+a.Path+": no target state in "+a.Detail)
				continue
			}
			if err := writeHeader(fs, src, speclife.State(a.Detail[i+len(detailArrow):])); err != nil {
				notes = append(notes, "docs sweep: header "+a.Path+": "+err.Error())
			}
			continue
		}
		dst := demotionPath(viewDir, a)
		if dst == "" {
			continue
		}
		if err := copyFile(fs, src, dst); err != nil {
			notes = append(notes, "docs sweep: keep "+a.Path+": "+err.Error())
			continue
		}
		if err := fs.Remove(src); err != nil {
			notes = append(notes, "docs sweep: remove "+a.Path+": "+err.Error())
		}
	}
	return notes
}

// demotionPath is where a deleted file's copy goes ("" for a non-delete kind).
func demotionPath(viewDir string, a Action) string {
	switch a.Kind {
	case KindDeleteReference:
		return filepath.Join(viewDir, "research", filepath.FromSlash(strings.TrimPrefix(a.Path, "reference/")))
	case KindDeleteHandoff:
		return filepath.Join(viewDir, "notes", filepath.FromSlash(strings.TrimPrefix(a.Path, "handoff/")))
	case KindDeletePlan:
		return filepath.Join(viewDir, "notes", filepath.FromSlash(a.Path))
	}
	return ""
}

func copyFile(fs FS, src, dst string) error {
	b, err := fs.ReadFile(src)
	if err != nil {
		return err
	}
	if err := fs.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return fs.WriteFile(freeName(fs, dst), b, 0o644)
}

// freeName returns dst, or dst with .1, .2, ... before the extension when dst exists, so a
// demotion copy never overwrites a local file that is not synced anywhere.
func freeName(fs FS, dst string) string {
	if _, err := fs.Stat(dst); err != nil {
		return dst
	}
	ext := filepath.Ext(dst)
	stem := strings.TrimSuffix(dst, ext)
	for i := 1; ; i++ {
		c := stem + "." + strconv.Itoa(i) + ext
		if _, err := fs.Stat(c); err != nil {
			return c
		}
	}
}

func writeHeader(fs FS, p string, state speclife.State) error {
	info, err := fs.Stat(p)
	if err != nil {
		return err
	}
	b, err := fs.ReadFile(p)
	if err != nil {
		return err
	}
	out, changed, _ := rewriteHeader(b, state)
	if !changed {
		return nil
	}
	if err := fs.WriteFile(p+".tmp", out, info.Mode().Perm()); err != nil {
		return err
	}
	return fs.Rename(p+".tmp", p)
}

// rewriteHeader sets a spec's status header to state. The first "**Trạng thái:**"/"**Status:**" //znf:allow-lang
// line has its value replaced (label kept); with none, a status line is inserted after the last
// "**Label:**" line before the first "---" or "## ", else after the first H1 framed by blank lines.
// note is the action detail ("<old> → <state>" / "insert → <state>") or, unchanged, why ("<no H1>").
func rewriteHeader(body []byte, state speclife.State) (out []byte, changed bool, note string) {
	lines := strings.Split(string(body), "\n")
	for i, l := range lines {
		m := statusLine.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		old := strings.TrimSpace(m[2])
		if old == string(state) {
			return body, false, ""
		}
		lines[i] = "**" + m[1] + ":** " + string(state)
		return []byte(strings.Join(lines, "\n")), true, old + detailArrow + string(state)
	}
	note = "insert" + detailArrow + string(state)
	status := statusLabel + string(state)
	lastLabel := -1
	for i, l := range lines {
		if l == "---" || strings.HasPrefix(l, "## ") {
			break
		}
		if labelLine.MatchString(l) {
			lastLabel = i
		}
	}
	if lastLabel >= 0 {
		return []byte(strings.Join(insertAt(lines, lastLabel+1, status), "\n")), true, note
	}
	for i, l := range lines {
		if !strings.HasPrefix(l, "# ") {
			continue
		}
		rest := lines[i+1:]
		if len(rest) > 0 && rest[0] == "" {
			rest = rest[1:] // reuse the existing blank line instead of doubling it
		}
		res := append(append([]string{}, lines[:i+1]...), "", status, "")
		res = append(res, rest...)
		return []byte(strings.Join(res, "\n")), true, note
	}
	return body, false, "<no H1>"
}

func insertAt(lines []string, i int, l string) []string {
	res := append(append([]string{}, lines[:i]...), l)
	return append(res, lines[i:]...)
}

// handoffDate returns the date of the first segment of rel that starts with a parseable
// YYYY-MM-DD; a date-like prefix that does not parse does not count.
func handoffDate(rel string) (time.Time, bool) {
	for _, seg := range strings.Split(rel, "/") {
		m := datePrefix.FindString(seg)
		if m == "" {
			continue
		}
		if d, err := time.Parse("2006-01-02", m); err == nil {
			return d, true
		}
	}
	return time.Time{}, false
}

// walkFiles lists the regular files under root, recursively, as "/"-paths relative to root.
// Entries whose name starts with "." are skipped (root itself may be a dot dir). A missing root
// is empty, not an error.
func walkFiles(fs FS, root string) ([]string, error) {
	if _, err := fs.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	var out []string
	var walk func(dir, rel string) error
	walk = func(dir, rel string) error {
		entries, err := fs.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			r := path.Join(rel, e.Name())
			switch {
			case e.IsDir():
				if err := walk(filepath.Join(dir, e.Name()), r); err != nil {
					return err
				}
			case e.Type().IsRegular():
				out = append(out, r)
			}
		}
		return nil
	}
	err := walk(root, "")
	return out, err
}

// planStem is a spec's mirror plan name: basename without ".md", then without "-design"
// (same derivation as the CLI's planExists).
func planStem(specPath string) string {
	base := strings.TrimSuffix(path.Base(specPath), ".md")
	return strings.TrimSuffix(base, "-design")
}
