package release

import (
	"errors"
	"strings"
	"testing"
)

// fakeRunner: khớp gitx.Runner, trả output cố định theo args (join bằng khoảng trắng).
// Dùng chung cho git_test / repos_test / release_test.
type fakeRunner struct {
	out map[string]string
	err map[string]string
}

func (f fakeRunner) Run(dir string, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	if f.err != nil {
		if e, ok := f.err[key]; ok {
			return nil, errors.New(e)
		}
	}
	return []byte(f.out[key]), nil
}

func TestReleaseNumsAndPrev(t *testing.T) {
	f := fakeRunner{out: map[string]string{
		"branch -r": "  origin/release83\n  origin/release84\n  origin/staging\n  origin/main\n",
	}}
	nums, err := ReleaseNums(f, "/x")
	if err != nil || len(nums) != 2 || nums[0] != 83 || nums[1] != 84 {
		t.Fatalf("nums=%v err=%v", nums, err)
	}
	if p, ok := PrevRelease(nums, 84); !ok || p != 83 {
		t.Fatalf("prev=%d ok=%v", p, ok)
	}
	if _, ok := PrevRelease([]int{84}, 84); ok {
		t.Fatal("no prev expected")
	}
}

func TestReleaseNumsIgnoresVariantRefs(t *testing.T) {
	f := fakeRunner{out: map[string]string{
		"branch -r": "  origin/release84-hotfix\n  origin/release83.1\n  origin/release82\n  origin/staging\n",
	}}
	nums, err := ReleaseNums(f, "/x")
	if err != nil || len(nums) != 1 || nums[0] != 82 {
		t.Fatalf("biến thể release84-hotfix/release83.1 phải bị bỏ: nums=%v err=%v", nums, err)
	}
}

func TestRangeCommits(t *testing.T) {
	f := fakeRunner{out: map[string]string{
		"log --format=%h\x1f%s origin/release83..origin/release84": "5ed5aa1\x1ffix: a\na8601ee\x1fMerge pull request #1 from org/hungnk/hotfix/x\n",
	}}
	cs, err := RangeCommits(f, "/x", "origin/release83", "origin/release84")
	if err != nil || len(cs) != 2 {
		t.Fatalf("cs=%v err=%v", cs, err)
	}
	if cs[0].Type != "fix" || cs[1].Merge != true || cs[1].Branch != "hungnk/hotfix/x" {
		t.Fatalf("bad parse: %+v", cs)
	}
}

func TestNotInStaging(t *testing.T) {
	f := fakeRunner{out: map[string]string{
		"log --format=%h\x1f%s origin/release83..origin/release84 --not origin/staging": "9dcc752\x1ftemporary disable report api\n",
	}}
	cs, err := NotInStaging(f, "/x", "origin/release83", "origin/release84", "origin/staging")
	if err != nil || len(cs) != 1 || cs[0].SHA != "9dcc752" {
		t.Fatalf("cs=%v err=%v", cs, err)
	}
}

func TestChangedFiles(t *testing.T) {
	f := fakeRunner{out: map[string]string{
		"diff --name-only origin/release83..origin/release84": "db/migrations/1.js\napp/x.js\n",
	}}
	fs, err := ChangedFiles(f, "/x", "origin/release83", "origin/release84")
	if err != nil || len(fs) != 2 || fs[0] != "db/migrations/1.js" {
		t.Fatalf("fs=%v err=%v", fs, err)
	}
}

func TestCutDate(t *testing.T) {
	f := fakeRunner{out: map[string]string{
		"merge-base origin/release84 origin/staging": "base1\n",
		"log -1 --format=%ci base1":                  "2026-08-26 17:55:55 +0700\n",
	}}
	d, err := CutDate(f, "/x", 84)
	if err != nil || d != "2026-08-26" {
		t.Fatalf("d=%q err=%v", d, err)
	}
}

func TestFetchPropagatesError(t *testing.T) {
	f := fakeRunner{err: map[string]string{"fetch origin release84 staging": "network down"}}
	if err := Fetch(f, "/x", "release84", "staging"); err == nil {
		t.Fatal("expected fetch error")
	}
}
