package docsgen

import (
	"bytes"
	"embed"
	"io/fs"
	"regexp"
	"strings"
)

// The catalog holds the hand-written Vietnamese prose for every reference
// page. The generator merges it with the mechanical facts taken from the
// binary (usage line, flags, hook table, skill invocation), so the prose can
// read like documentation while the facts can never drift.
//
// Fragment layout (catalog/<kind>/<page>.md):
//
//	---
//	summary: one sentence shown under the title and in the index table
//	---
//	## Khi nào dùng      // a Vietnamese H2 written by the fragment author //znf:allow-lang
//	...
//
// A fragment may carry its own hand-written flags table; a test then checks
// that every flag the binary declares is mentioned in it.
//
//go:embed all:catalog
var catalogFS embed.FS

// Fragment is one parsed catalog entry.
type Fragment struct {
	Summary string
	Body    string // markdown after the frontmatter, trimmed
}

var summaryRe = regexp.MustCompile(`(?m)^summary:\s*(.+)$`)

// LoadFragment reads catalog/<rel>.md. ok=false when the page has no entry.
func LoadFragment(rel string) (Fragment, bool) {
	b, err := fs.ReadFile(catalogFS, "catalog/"+rel+".md")
	if err != nil {
		return Fragment{}, false
	}
	return parseFragment(b), true
}

func parseFragment(b []byte) Fragment {
	var f Fragment
	body := b
	if bytes.HasPrefix(b, fmDelim) {
		rest := b[len(fmDelim):]
		if end := bytes.Index(rest, []byte("\n---")); end >= 0 {
			if m := summaryRe.FindSubmatch(rest[:end]); m != nil {
				f.Summary = strings.TrimSpace(string(m[1]))
			}
			body = rest[end+len("\n---"):]
		}
	}
	f.Body = strings.TrimSpace(string(body))
	return f
}

// HasSection reports whether the fragment body carries a given H2.
func (f Fragment) HasSection(h2 string) bool {
	return strings.Contains("\n"+f.Body+"\n", "\n## "+h2+"\n")
}

// CatalogPages lists every fragment path (kind/page) present in the catalog.
func CatalogPages() []string {
	var out []string
	_ = fs.WalkDir(catalogFS, "catalog", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".md") {
			out = append(out, strings.TrimSuffix(strings.TrimPrefix(p, "catalog/"), ".md"))
		}
		return nil
	})
	return out
}
