package cli

import (
	"os"
	"path/filepath"
)

// resolveDocsStore trả path repo store THẬT (chứa .git/.config/.claude).
// Thứ tự resolve (dừng ở cái đầu tiên TỒN TẠI): $ZENIFY_HOME/knowledge →
// ~/.zenify/knowledge → fallback workspace docs (máy chưa migrate). Nếu không
// cái nào tồn tại → trả đường CHUẨN (env hoặc ~/.zenify/knowledge) để onboarding
// clone tạo. Toàn bộ I/O inject để test thuần.
func resolveDocsStore(workspaceDir string, getenv func(string) string, userHome func() (string, error), stat func(string) (os.FileInfo, error), readDir func(string) ([]os.DirEntry, error)) string {
	home, _ := userHome()
	target := filepath.Join(home, ".zenify", "knowledge")
	if zh := getenv("ZENIFY_HOME"); zh != "" {
		target = filepath.Join(zh, "knowledge")
	}
	if isDirStat(stat, target) {
		return target
	}
	ws := resolveWorkspaceRepoDir(workspaceDir, defaultDocsRepo, "", readDir)
	if isDirStat(stat, ws) {
		return ws
	}
	return target
}

func isDirStat(stat func(string) (os.FileInfo, error), p string) bool {
	fi, err := stat(p)
	return err == nil && fi.IsDir()
}
