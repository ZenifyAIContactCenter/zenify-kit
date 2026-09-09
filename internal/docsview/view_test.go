package docsview

import (
	"os"
	"strings"
	"testing"
)

// fakeFS: store has specs/ plans/ .config/ .git/ README.md ; view holds links (map link→target).
// "managed link" = key present in links. "same target" = links[link]==target AND target is
// still in wantExists.
type fakeFS struct {
	dirs       map[string][]string // path → child entry names
	isDir      map[string]bool     // entry name → is a dir?
	links      map[string]string   // link path → target path
	targetGone map[string]bool     // target path is gone (for testing a dead link)
}

func (f *fakeFS) ReadDir(p string) ([]os.DirEntry, error) {
	names, ok := f.dirs[p]
	if !ok {
		return nil, os.ErrNotExist
	}
	var out []os.DirEntry
	for _, n := range names {
		out = append(out, dentry{n, f.isDir[n]})
	}
	return out, nil
}
func (f *fakeFS) MkdirAll(string, os.FileMode) error   { return nil }
func (f *fakeFS) Remove(p string) error                { delete(f.links, p); return nil }
func (f *fakeFS) Link(target, link string) error       { f.links[link] = target; return nil }
func (f *fakeFS) IsManagedLink(p string) (bool, error) { _, ok := f.links[p]; return ok, nil }
func (f *fakeFS) SameTarget(link, target string) (bool, error) {
	cur, ok := f.links[link]
	if !ok {
		return false, os.ErrNotExist
	}
	if f.targetGone[cur] {
		return false, nil
	} // target gone → dead link
	return cur == target, nil
}

type dentry struct {
	n string
	d bool
}

func (d dentry) Name() string               { return d.n }
func (d dentry) IsDir() bool                { return d.d }
func (d dentry) Type() os.FileMode          { return 0 }
func (d dentry) Info() (os.FileInfo, error) { return nil, nil }

func TestEnsureView_LinksOnlyNonDotDirs(t *testing.T) {
	fs := &fakeFS{
		dirs:  map[string][]string{"/store": {"specs", "plans", ".config", ".git", "README.md"}, "/view": {}},
		isDir: map[string]bool{"specs": true, "plans": true, ".config": true, ".git": true, "README.md": false},
		links: map[string]string{}, targetGone: map[string]bool{},
	}
	EnsureView(fs, "/store", "/view")
	if fs.links["/view/specs"] != "/store/specs" || fs.links["/view/plans"] != "/store/plans" {
		t.Fatalf("missing content link: %v", fs.links)
	}
	for bad := range fs.links {
		if strings.Contains(bad, ".config") || strings.Contains(bad, ".git") || strings.Contains(bad, "README") {
			t.Fatalf("must NOT link a dotfile/file: %s", bad)
		}
	}
}

func TestEnsureView_FixesWrongTarget(t *testing.T) {
	fs := &fakeFS{
		dirs:  map[string][]string{"/store": {"specs"}, "/view": {"specs"}},
		isDir: map[string]bool{"specs": true},
		links: map[string]string{"/view/specs": "/OLD/specs"}, targetGone: map[string]bool{},
	}
	EnsureView(fs, "/store", "/view")
	if fs.links["/view/specs"] != "/store/specs" {
		t.Fatalf("wrong target not fixed: %v", fs.links)
	}
}

func TestEnsureView_PrunesDeadLink(t *testing.T) {
	fs := &fakeFS{
		dirs:       map[string][]string{"/store": {"specs"}, "/view": {"specs", "gone"}},
		isDir:      map[string]bool{"specs": true},
		links:      map[string]string{"/view/specs": "/store/specs", "/view/gone": "/store/gone"},
		targetGone: map[string]bool{"/store/gone": true},
	}
	EnsureView(fs, "/store", "/view")
	if _, ok := fs.links["/view/gone"]; ok {
		t.Fatalf("dead link was not pruned")
	}
}
