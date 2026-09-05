package release

import (
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// Resolve trả danh sách repo được-theo-dõi-release. Nếu <workspace>/.znf/release-repos.txt
// đọc được → dùng các dòng non-empty. Ngược lại auto-detect: subdir nào có origin/release<n>.
// readFile/readDir được inject để test (CLI truyền os.ReadFile / os.ReadDir).
func Resolve(r gitx.Runner, workspace string, n int,
	readFile func(string) ([]byte, error), readDir func(string) ([]string, error)) ([]string, error) {

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
	dirs, err := readDir(workspace)
	if err != nil {
		return nil, err
	}
	var repos []string
	for _, d := range dirs {
		nums, err := ReleaseNums(r, filepath.Join(workspace, d))
		if err != nil {
			continue
		}
		for _, x := range nums {
			if x == n {
				repos = append(repos, d)
				break
			}
		}
	}
	return repos, nil
}
