// Package workspace định vị các git repo trong một workspace theo độ sâu,
// để consumer không phải giả định "repo = con trực tiếp của root". Thuần: readDir inject.
package workspace

import (
	"os"
	"path/filepath"
	"strings"
)

const DefaultMaxDepth = 2

// Repo là một git repo tìm được. Name = basename, Path = đường dẫn tới thư mục repo.
type Repo struct {
	Name string
	Path string
}

// isRepo: thư mục chứa entry ".git" dạng THƯ MỤC (main checkout), HOẶC chứa
// ".claude/worktree.json". Linked worktree có ".git" là file → không đếm là repo.
// Đọc qua readDir được inject, không gọi thẳng os.Stat, để giữ hàm thuần/testable.
func isRepo(dir string, entries []os.DirEntry, readDir func(string) ([]os.DirEntry, error)) bool {
	for _, e := range entries {
		if e.Name() == ".git" && e.IsDir() {
			return true
		}
	}
	hasClaudeDir := false
	for _, e := range entries {
		if e.Name() == ".claude" && e.IsDir() {
			hasClaudeDir = true
			break
		}
	}
	if !hasClaudeDir {
		return false
	}
	claudeEntries, err := readDir(filepath.Join(dir, ".claude"))
	if err != nil {
		return false
	}
	for _, e := range claudeEntries {
		if e.Name() == "worktree.json" && !e.IsDir() {
			return true
		}
	}
	return false
}

// Discover walk từ root tới độ sâu maxDepth; nhận repo thì DỪNG đệ quy nhánh đó.
// Bỏ qua thư mục ẩn (.git, .worktrees, .claude) và node_modules để không lạc vào trong repo.
func Discover(root string, maxDepth int, readDir func(string) ([]os.DirEntry, error)) []Repo {
	var out []Repo
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > maxDepth {
			return
		}
		entries, err := readDir(dir)
		if err != nil {
			return
		}
		if depth >= 1 && isRepo(dir, entries, readDir) {
			out = append(out, Repo{Name: filepath.Base(dir), Path: dir})
			return
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" {
				continue
			}
			walk(filepath.Join(dir, e.Name()), depth+1)
		}
	}
	walk(root, 0)
	return out
}

// Resolve tìm repo theo name; trùng tên nhiều nơi → trả cái nông nhất (ít segment path nhất).
func Resolve(root, name string, maxDepth int, readDir func(string) ([]os.DirEntry, error)) (string, bool) {
	best := ""
	bestDepth := 1 << 30
	for _, r := range Discover(root, maxDepth, readDir) {
		if r.Name != name {
			continue
		}
		d := strings.Count(r.Path, string(filepath.Separator))
		if d < bestDepth {
			best, bestDepth = r.Path, d
		}
	}
	return best, best != ""
}
