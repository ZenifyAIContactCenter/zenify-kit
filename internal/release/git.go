package release

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

const sep = "\x1f" // unit separator giữa sha và subject trong --format

// Chốt cuối là whitespace/EOL để KHÔNG khớp biến thể như origin/release84-hotfix hay release84.1.
var relNumRe = regexp.MustCompile(`(?m)origin/release(\d+)(?:\s|$)`)

// ReleaseNums liệt kê các số release từ `git branch -r` (chỉ origin/release<N>), tăng dần.
func ReleaseNums(r gitx.Runner, dir string) ([]int, error) {
	out, err := r.Run(dir, "branch", "-r")
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	for _, m := range relNumRe.FindAllStringSubmatch(string(out), -1) {
		n, _ := strconv.Atoi(m[1])
		seen[n] = true
	}
	var nums []int
	for n := range seen {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	return nums, nil
}

// PrevRelease trả số release cao nhất < n trong nums.
func PrevRelease(nums []int, n int) (int, bool) {
	prev, ok := 0, false
	for _, x := range nums {
		if x < n && x > prev {
			prev, ok = x, true
		}
	}
	return prev, ok
}

// Fetch chạy `git fetch origin <refs...>`.
func Fetch(r gitx.Runner, dir string, refs ...string) error {
	args := append([]string{"fetch", "origin"}, refs...)
	_, err := r.Run(dir, args...)
	return err
}

func parseCommits(out []byte) []Commit {
	var cs []Commit
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, sep, 2)
		if len(parts) != 2 {
			continue
		}
		c := Commit{SHA: parts[0], Subject: parts[1], Type: ClassifyType(parts[1])}
		if b := ParseMergeBranch(parts[1]); b != "" {
			c.Merge, c.Branch = true, b
		}
		cs = append(cs, c)
	}
	return cs
}

// RangeCommits trả các commit trong from..to (giữ cả merge để bắt PR/hotfix).
func RangeCommits(r gitx.Runner, dir, from, to string) ([]Commit, error) {
	out, err := r.Run(dir, "log", "--format=%h"+sep+"%s", from+".."+to)
	if err != nil {
		return nil, err
	}
	return parseCommits(out), nil
}

// ChangedFiles trả danh sách file đổi trong from..to.
func ChangedFiles(r gitx.Runner, dir, from, to string) ([]string, error) {
	out, err := r.Run(dir, "diff", "--name-only", from+".."+to)
	if err != nil {
		return nil, err
	}
	var fs []string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if l != "" {
			fs = append(fs, l)
		}
	}
	return fs, nil
}

// NotInStaging trả các commit trong from..to KHÔNG reachable từ staging (rủi ro regression).
func NotInStaging(r gitx.Runner, dir, from, to, staging string) ([]Commit, error) {
	out, err := r.Run(dir, "log", "--format=%h"+sep+"%s", from+".."+to, "--not", staging)
	if err != nil {
		return nil, err
	}
	return parseCommits(out), nil
}

// CutDate trả ngày (YYYY-MM-DD) của merge-base giữa release<n> và origin/staging.
func CutDate(r gitx.Runner, dir string, n int) (string, error) {
	rel := fmt.Sprintf("origin/release%d", n)
	base, err := r.Run(dir, "merge-base", rel, "origin/staging")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(base))
	out, err := r.Run(dir, "log", "-1", "--format=%ci", sha)
	if err != nil {
		return "", err
	}
	d := strings.TrimSpace(string(out))
	if len(d) >= 10 {
		d = d[:10]
	}
	return d, nil
}
