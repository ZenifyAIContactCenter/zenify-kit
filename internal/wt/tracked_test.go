package wt

import (
	"reflect"
	"testing"
)

func TestDropTrackedInBase_SplitsTrackedFromUntracked(t *testing.T) {
	root := "/repo"
	r := fakeRunner{
		out: map[string]string{root + "|cat-file -e main:CLAUDE.md": ""},
		err: map[string]error{root + "|cat-file -e main:.claude/settings.local.json": errFake},
	}
	keep, skipped := dropTrackedInBase(r, root, "main", []string{"CLAUDE.md", " .claude/settings.local.json ", ""})
	if !reflect.DeepEqual(keep, []string{".claude/settings.local.json"}) {
		t.Fatalf("keep = %v", keep)
	}
	if !reflect.DeepEqual(skipped, []string{"CLAUDE.md"}) {
		t.Fatalf("skipped = %v", skipped)
	}
}

func TestDropTrackedInBase_DirectoryEntryTrimsSlash(t *testing.T) {
	root := "/repo"
	r := fakeRunner{out: map[string]string{root + "|cat-file -e main:docs": ""}}
	_, skipped := dropTrackedInBase(r, root, "main", []string{"docs/"})
	if !reflect.DeepEqual(skipped, []string{"docs/"}) {
		t.Fatalf("directory entry must be checked without its trailing slash, skipped = %v", skipped)
	}
}

func TestDropTrackedInBase_GitErrorFailsOpenToSeed(t *testing.T) {
	root := "/repo"
	r := fakeRunner{err: map[string]error{root + "|cat-file -e main:x.txt": errFake}}
	keep, skipped := dropTrackedInBase(r, root, "main", []string{"x.txt"})
	if len(skipped) != 0 || !reflect.DeepEqual(keep, []string{"x.txt"}) {
		t.Fatalf("a git error must keep the entry (seed as before), keep=%v skipped=%v", keep, skipped)
	}
}
