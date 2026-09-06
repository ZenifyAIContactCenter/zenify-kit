package docsview

import (
	"os"
	"path/filepath"
	"strings"
)

// FS gói mọi thao tác OS-specific SAU seam để EnsureView OS-agnostic.
// OSFS (Task 3) cài: unix dùng os.Symlink; windows dùng directory junction (mklink /J).
type FS interface {
	ReadDir(string) ([]os.DirEntry, error)
	MkdirAll(string, os.FileMode) error
	Remove(string) error
	// Link tạo link từ link→target (symlink unix / junction windows).
	Link(target, link string) error
	// IsManagedLink: path là link mình quản (symlink/junction), KHÔNG phải dir/file thật?
	IsManagedLink(path string) (bool, error)
	// SameTarget: link có resolve về đúng target? (EvalSymlinks compare; lỗi nếu link chết).
	SameTarget(link, target string) (bool, error)
}

// EnsureView đảm bảo viewDir chỉ chứa link tới mỗi top-level DIR không-chấm của store.
// Idempotent: tạo thiếu, sửa sai target, dọn link chết. KHÔNG đụng dir/file THẬT trong
// viewDir (chỉ thao tác link mình quản). Fail-open: lỗi → note, không panic.
func EnsureView(fs FS, store, viewDir string) []string {
	var notes []string
	entries, err := fs.ReadDir(store)
	if err != nil {
		return []string{"docs view: không đọc được store " + store + ": " + err.Error()}
	}
	if err := fs.MkdirAll(viewDir, 0o755); err != nil {
		return []string{"docs view: mkdir viewDir lỗi: " + err.Error()}
	}
	want := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		want[e.Name()] = true
		link := filepath.Join(viewDir, e.Name())
		target := filepath.Join(store, e.Name())
		if managed, _ := fs.IsManagedLink(link); managed {
			if same, _ := fs.SameTarget(link, target); same {
				continue // đã đúng
			}
			_ = fs.Remove(link) // link sai target → gỡ, tạo lại (chỉ gỡ 1 link, an toàn với junction)
		} else if _, err := fs.ReadDir(link); err == nil {
			// có dir/file THẬT chiếm chỗ → KHÔNG clobber, chỉ note
			notes = append(notes, "docs view: "+link+" là dir/file thật (không phải link) — bỏ qua")
			continue
		}
		if err := fs.Link(target, link); err != nil {
			notes = append(notes, "docs view: tạo link "+link+" lỗi: "+err.Error())
		}
	}
	if vents, err := fs.ReadDir(viewDir); err == nil {
		for _, v := range vents {
			if want[v.Name()] {
				continue
			}
			link := filepath.Join(viewDir, v.Name())
			if managed, _ := fs.IsManagedLink(link); managed {
				_ = fs.Remove(link) // KHÔNG RemoveAll: chỉ gỡ link, không xoá nội dung store
				notes = append(notes, "docs view: dọn link chết "+v.Name())
			}
		}
	}
	return notes
}
