package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// gitRepo creates a real temp git repo isolated from the machine's git config, so a signing or
// hook setting on the dev machine cannot hang or fail the test.
func gitRepo(t *testing.T) (dir string, git func(args ...string) string) {
	t.Helper()
	dir = t.TempDir()
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	git = func(args ...string) string {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t",
			"-c", "commit.gpgsign=false", "-c", "tag.gpgSign=false"}, args...)
		out, err := exec.Command("git", full...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	git("init", "-q", "-b", "main")
	return dir, git
}

// Tags out of creation order, with lookalikes that are NOT release tags (semver, rc, legacy branch
// name). Expected order is by date then number: .10 sorts after .2, not lexically before it.
func TestReleaseTagsRealRepo(t *testing.T) {
	dir, git := gitRepo(t)
	git("commit", "-q", "--allow-empty", "-m", "c1")
	for _, tag := range []string{"v261104.0", "v261028.10", "v261028.0", "v261028.2", "v0.52.0", "v261028.0-rc", "release84"} {
		git("tag", tag)
	}
	got, err := ReleaseTags(dir)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := []string{"v261028.0", "v261028.2", "v261028.10", "v261104.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReleaseTags = %v, want %v", got, want)
	}
}

func TestReleaseTagsNoTags(t *testing.T) {
	dir, git := gitRepo(t)
	git("commit", "-q", "--allow-empty", "-m", "c1")
	got, err := ReleaseTags(dir)
	if err != nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v, want no tags", got, err)
	}
}

func TestPrevTag(t *testing.T) {
	tags := []string{"v261028.0", "v261028.2", "v261104.0"}
	if p, ok := PrevTag(tags, "v261104.0"); !ok || p != "v261028.2" {
		t.Fatalf("PrevTag(v261104.0) = %q,%v want v261028.2", p, ok)
	}
	// hotfix tag on the same day: its predecessor is the weekly tag of that day.
	if p, ok := PrevTag(tags, "v261028.2"); !ok || p != "v261028.0" {
		t.Fatalf("PrevTag(v261028.2) = %q,%v want v261028.0", p, ok)
	}
	if _, ok := PrevTag(tags, "v261028.0"); ok {
		t.Fatal("the first tag has no predecessor")
	}
}

// Build over a real repo: the range is prevTag..tag (hotfix tag included as the lower bound),
// the default target is the repo's newest tag, and a repo with no tag gets a "chưa có tag" note. //znf:allow-lang
func TestBuildByTagRealRepos(t *testing.T) {
	be, git := gitRepo(t)
	git("commit", "-q", "--allow-empty", "-m", "feat(a): weekly one")
	git("tag", "v261028.0")
	git("commit", "-q", "--allow-empty", "-m", "fix(b): hotfix midweek")
	git("tag", "v261028.2")
	git("commit", "-q", "--allow-empty", "-m", "feat(c): next week")
	git("tag", "v261104.0")
	bare, git2 := gitRepo(t)
	git2("commit", "-q", "--allow-empty", "-m", "init")
	// old: its newest tag is older than the workspace's newest → omitted from the default report.
	old, git3 := gitRepo(t)
	git3("commit", "-q", "--allow-empty", "-m", "feat(o): old weekly")
	git3("tag", "v261028.0")
	git3("commit", "-q", "--allow-empty", "-m", "fix(o): old hotfix")
	git3("tag", "v261028.2")

	dirs := map[string]string{"be": be, "bare": bare, "old": old}
	resolve := func(n string) (string, bool) { d, ok := dirs[n]; return d, ok }
	none := func(string) []string { return nil }
	noSpecs := func(string) []SpecMeta { return nil }

	rep := Build(gitx.ExecRunner(), resolve, []string{"be", "old", "bare"}, "", none, noSpecs)
	if rep.Tag != "v261104.0" {
		t.Fatalf("report tag = %q, want v261104.0", rep.Tag)
	}
	// default = newest workspace tag; "old" does not carry it → omitted (no row), "bare" keeps its note.
	if len(rep.Repos) != 2 || rep.Repos[0].Name != "be" || rep.Repos[1].Name != "bare" {
		t.Fatalf("repos = %+v", rep.Repos)
	}
	r0 := rep.Repos[0]
	if r0.Err != "" || r0.Tag != "v261104.0" || r0.PrevTag != "v261028.2" {
		t.Fatalf("be section = %+v", r0)
	}
	if len(r0.Commits) != 1 || r0.Commits[0].Subject != "feat(c): next week" {
		t.Fatalf("be commits in v261028.2..v261104.0 = %+v", r0.Commits)
	}
	if rep.Repos[1].Err != "chưa có tag vYYMMDD.n" { //znf:allow-lang
		t.Fatalf("untagged repo note = %q", rep.Repos[1].Err)
	}

	// Explicit hotfix tag: range is the weekly tag of that day up to the hotfix.
	rep = Build(gitx.ExecRunner(), resolve, []string{"be"}, "v261028.2", none, noSpecs)
	if rep.Tag != "v261028.2" || len(rep.Repos[0].Commits) != 1 || rep.Repos[0].Commits[0].Subject != "fix(b): hotfix midweek" {
		t.Fatalf("hotfix report = %+v", rep.Repos[0])
	}

	// Explicit tag: a repo that does not carry it keeps its note.
	rep = Build(gitx.ExecRunner(), resolve, []string{"old"}, "v261104.0", none, noSpecs)
	if len(rep.Repos) != 1 || rep.Repos[0].Err != "chưa có tag v261104.0" { //znf:allow-lang
		t.Fatalf("explicit mode must keep the missing-tag row: %+v", rep.Repos)
	}

	// Explicit tag a repo does not carry → note, not a silent skip.
	rep = Build(gitx.ExecRunner(), resolve, []string{"be"}, "v261111.0", none, noSpecs)
	if rep.Repos[0].Err != "chưa có tag v261111.0" { //znf:allow-lang
		t.Fatalf("missing tag note = %q", rep.Repos[0].Err)
	}
}

// Team flow since 2026-10-28: feature PRs merge into staging, the weekly release is a PR
// staging→main tagged vYYMMDD.n, a hotfix is a PR straight into main then synced main→staging.
// The release merge must expand into its inner PRs (no "staging" row), the hotfix keeps its own
// branch, and the main→staging sync must not drag the already-shipped hotfix into the next week.
func TestRangeCommitsGroupedExpandsReleaseMerge(t *testing.T) {
	dir, git := gitRepo(t)
	git("commit", "-q", "--allow-empty", "-m", "base")
	git("tag", "v261028.0")
	git("checkout", "-q", "-b", "staging")
	pr := func(into, branch, num, subject string) {
		git("checkout", "-q", "-b", branch, into)
		git("commit", "-q", "--allow-empty", "-m", subject)
		git("checkout", "-q", into)
		git("merge", "-q", "--no-ff", "-m", "Merge pull request #"+num+" from Org/"+branch, branch)
	}
	pr("staging", "u/feat/foo", "11", "feat(foo): add foo")
	pr("staging", "u/fix/bar", "12", "fix(bar): fix bar")
	git("checkout", "-q", "main")
	git("merge", "-q", "--no-ff", "-m", "Merge pull request #100 from Org/staging", "staging")
	git("tag", "v261104.0")
	pr("main", "u/hotfix/baz", "13", "fix(baz): hotfix baz")
	git("tag", "v261104.1")
	git("checkout", "-q", "staging")
	git("merge", "-q", "--no-ff", "-m", "Merge pull request #14 from Org/main", "main")
	pr("staging", "u/feat/qux", "15", "feat(qux): add qux")
	git("checkout", "-q", "main")
	git("merge", "-q", "--no-ff", "-m", "Merge pull request #101 from Org/staging", "staging")
	git("tag", "v261111.0")

	branches := func(from, to string) map[string]int {
		t.Helper()
		cs, err := RangeCommitsGrouped(gitx.ExecRunner(), dir, from, to)
		if err != nil {
			t.Fatalf("RangeCommitsGrouped(%s..%s) err = %v", from, to, err)
		}
		got := map[string]int{}
		for _, c := range cs {
			got[c.PRBranch]++
		}
		return got
	}
	if got, want := branches("v261028.0", "v261104.0"), map[string]int{"u/feat/foo": 2, "u/fix/bar": 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("weekly release PRBranch counts = %v, want %v", got, want)
	}
	if got, want := branches("v261104.0", "v261104.1"), map[string]int{"u/hotfix/baz": 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("hotfix PRBranch counts = %v, want %v", got, want)
	}
	if got := branches("v261104.1", "v261111.0"); got["u/hotfix/baz"] != 0 || got["staging"] != 0 || got["u/feat/qux"] != 2 {
		t.Fatalf("next weekly release PRBranch counts = %v, want u/feat/qux:2 and no hotfix/staging", got)
	}
}
