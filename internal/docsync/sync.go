// Package docsync đồng bộ repo docs (agent-managed): pull, rồi commit+push
// thay đổi agent ghi. Model Obsidian git-sync. Thuần: inject gitx.Runner.
package docsync

import (
	"fmt"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// Sync đồng bộ repo docs tại dir. FAIL-OPEN: luôn trả notes, không error.
// Idempotent: working tree sạch → không tạo empty commit. Read-only 0444
// (Phase 2) là lớp chặn dev-edit; ở đây dirty được coi là agent ghi.
func Sync(r gitx.Runner, dir string) []string {
	var notes []string
	if _, err := r.Run(dir, "pull", "--rebase", "--autostash"); err != nil {
		notes = append(notes, fmt.Sprintf("docs sync: pull lỗi: %v (fail-open)", err))
	}
	st, err := r.Run(dir, "status", "--porcelain")
	if err != nil {
		return append(notes, fmt.Sprintf("docs sync: status lỗi: %v (fail-open)", err))
	}
	if strings.TrimSpace(string(st)) == "" {
		return append(notes, "docs sync: clean")
	}
	if _, err := r.Run(dir, "add", "-A"); err != nil {
		return append(notes, fmt.Sprintf("docs sync: add lỗi: %v (fail-open)", err))
	}
	msg := "chore(docs): sync " + time.Now().UTC().Format("2006-01-02T15:04:05Z")
	if _, err := r.Run(dir, "commit", "-m", msg); err != nil {
		return append(notes, fmt.Sprintf("docs sync: commit lỗi: %v (fail-open)", err))
	}
	if _, err := r.Run(dir, "push"); err != nil {
		return append(notes, fmt.Sprintf("docs sync: push lỗi: %v (fail-open)", err))
	}
	return append(notes, "docs sync: pushed")
}
