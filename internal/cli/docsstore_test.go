package cli

import (
	"os"
	"testing"
	"time"
)

type fakeInfo struct{ dir bool }

func (fakeInfo) Name() string       { return "" }
func (fakeInfo) Size() int64        { return 0 }
func (fakeInfo) Mode() os.FileMode  { return 0 }
func (fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool      { return f.dir }
func (fakeInfo) Sys() any           { return nil }

func TestResolveDocsStore(t *testing.T) {
	home := "/h"
	uh := func() (string, error) { return home, nil }
	rd := func(string) ([]os.DirEntry, error) { return nil, nil }

	// 1) $ZENIFY_HOME tồn tại → thắng
	env := func(k string) string {
		if k == "ZENIFY_HOME" {
			return "/z"
		}
		return ""
	}
	st := func(p string) (os.FileInfo, error) {
		if p == "/z/knowledge" {
			return fakeInfo{true}, nil
		}
		return nil, os.ErrNotExist
	}
	if got := resolveDocsStore("/ws", env, uh, st, rd); got != "/z/knowledge" {
		t.Fatalf("env store: got %q", got)
	}
	// 2) không env, ~/.zenify/knowledge tồn tại
	env0 := func(string) string { return "" }
	st2 := func(p string) (os.FileInfo, error) {
		if p == "/h/.zenify/knowledge" {
			return fakeInfo{true}, nil
		}
		return nil, os.ErrNotExist
	}
	if got := resolveDocsStore("/ws", env0, uh, st2, rd); got != "/h/.zenify/knowledge" {
		t.Fatalf("home store: got %q", got)
	}
	// 3) chưa migrate: chỉ workspace/docs tồn tại → fallback
	st3 := func(p string) (os.FileInfo, error) {
		if p == "/ws/docs" {
			return fakeInfo{true}, nil
		}
		return nil, os.ErrNotExist
	}
	if got := resolveDocsStore("/ws", env0, uh, st3, rd); got != "/ws/docs" {
		t.Fatalf("fallback ws: got %q", got)
	}
	// 4) không cái nào tồn tại → đường chuẩn ~/.zenify/knowledge (để clone tạo)
	stN := func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	if got := resolveDocsStore("/ws", env0, uh, stN, rd); got != "/h/.zenify/knowledge" {
		t.Fatalf("target when none: got %q", got)
	}
}
