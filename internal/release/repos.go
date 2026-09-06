package release

import (
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	wspkg "github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
)

// Resolve trả danh sách repo được-theo-dõi-release. Nếu <workspace>/.znf/release-repos.txt
// đọc được → dùng các dòng non-empty. Ngược lại auto-detect trên `discovered` (kết quả
// workspace.Discover) — repo nào có origin/release<n>. readFile inject để test
// (CLI truyền os.ReadFile).
func Resolve(r gitx.Runner, workspace string, n int,
	readFile func(string) ([]byte, error), discovered []wspkg.Repo) ([]string, error) {

	if b, err := readFile(filepath.Join(workspace, ".znf", "release-repos.txt")); err == nil {
		var repos []string
		for _, l := range strings.Split(string(b), "\n") {
			s := strings.TrimSpace(l)
			if s == "" || strings.HasPrefix(s, "#") {
				continue
			}
			repos = append(repos, s)
		}
		return repos, nil
	}
	var repos []string
	for _, rp := range discovered {
		nums, err := ReleaseNums(r, rp.Path)
		if err != nil {
			continue
		}
		for _, x := range nums {
			if x == n {
				repos = append(repos, rp.Name)
				break
			}
		}
	}
	return repos, nil
}
