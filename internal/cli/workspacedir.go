package cli

import (
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
)

// resolveWorkspaceRepoDir trả <đường-dẫn-repo>/sub, tìm repo theo tên qua
// workspace.Discover — nên đúng cho CẢ layout phẳng (<ws>/<repo>) lẫn layout
// chuẩn team sau `zenify migrate` (<ws>/repos/<repo>). Không tìm thấy → fallback
// <ws>/<repo>/sub (giữ tương thích cũ; caller đã fail-open với path không tồn tại).
// readDir inject để test thuần được.
func resolveWorkspaceRepoDir(workspaceDir, repo, sub string, readDir func(string) ([]os.DirEntry, error)) string {
	base := filepath.Join(workspaceDir, repo)
	if p, ok := workspace.Resolve(workspaceDir, repo, workspace.DefaultMaxDepth, readDir); ok {
		base = p
	}
	return filepath.Join(base, sub)
}
