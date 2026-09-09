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

	if pin, ok := pinList(workspace, readFile); ok {
		return pin, nil
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

// ResolveUnreleased trả repo cho VIEW UNRELEASED. Vẫn ưu tiên pin `.znf/release-repos.txt` (nếu có).
// Auto-detect KHÁC Resolve: lấy MỌI repo có ÍT NHẤT một nhánh release (không đòi release<n>) — vì
// unreleased là "pending deploy hằng ngày" của mọi repo deploy, không phụ thuộc repo đã cắt release
// tuần này chưa. Repo có commit staging chưa deploy sẽ hiện; repo không có gì pending bị buildReport
// bỏ (section rỗng).
func ResolveUnreleased(r gitx.Runner, workspace string,
	readFile func(string) ([]byte, error), discovered []wspkg.Repo) ([]string, error) {

	if pin, ok := pinList(workspace, readFile); ok {
		return pin, nil
	}
	var repos []string
	for _, rp := range discovered {
		if nums, err := ReleaseNums(r, rp.Path); err == nil && len(nums) > 0 {
			repos = append(repos, rp.Name)
		}
	}
	return repos, nil
}

// pinList đọc `.znf/release-repos.txt` (một repo/dòng, `#`=comment). ok=false nếu file không đọc
// được (→ caller auto-detect). ⚠ File TỒN TẠI nhưng chỉ comment → ok=true với list RỖNG (quét 0
// repo) — có chủ đích: pin rỗng nghĩa là "không repo nào", muốn auto-detect thì XOÁ file.
func pinList(workspace string, readFile func(string) ([]byte, error)) ([]string, bool) {
	b, err := readFile(filepath.Join(workspace, ".znf", "release-repos.txt"))
	if err != nil {
		return nil, false
	}
	var repos []string
	for _, l := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(l)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		repos = append(repos, s)
	}
	return repos, true
}
