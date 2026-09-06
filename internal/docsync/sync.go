// Package docsync đồng bộ repo docs (agent-managed): commit thay đổi agent
// ghi, rồi rebase lên remote và push. Model Obsidian git-sync. Thuần: inject
// gitx.Runner.
package docsync

import (
	"fmt"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// Sync đồng bộ repo docs tại dir. FAIL-OPEN: luôn trả notes, không error.
//
// Thứ tự cố ý (status-first, commit-first):
//  1. status --porcelain — LOCAL, không network. Sạch → trả ngay. Đa số turn
//     (hook Stop chạy mỗi turn) sạch nên KHÔNG tốn round-trip mạng nào.
//  2. Dirty → add -A + commit (agent ghi giờ đã là commit, không còn dirty).
//  3. pull --rebase — rebase commit của ta lên remote. Nếu conflict, đây là
//     một rebase THẬT nên abort được: `rebase --abort` khôi phục commit của ta
//     nguyên vẹn (chỉ chưa push), và ta KHÔNG bao giờ add/commit đè lên
//     conflict marker rồi push rác lên repo chung.
//  4. push.
func Sync(r gitx.Runner, dir string) []string {
	st, err := r.Run(dir, "status", "--porcelain")
	if err != nil {
		return note(fmt.Sprintf("docs sync: status lỗi: %v (fail-open)", err))
	}
	if strings.TrimSpace(string(st)) == "" {
		return note("docs sync: clean") // không network, không commit
	}
	if _, err := r.Run(dir, "add", "-A"); err != nil {
		return note(fmt.Sprintf("docs sync: add lỗi: %v (fail-open)", err))
	}
	msg := "chore(docs): sync " + time.Now().UTC().Format("2006-01-02T15:04:05Z")
	if _, err := r.Run(dir, "commit", "-m", msg); err != nil {
		return note(fmt.Sprintf("docs sync: commit lỗi: %v (fail-open)", err))
	}
	if _, err := r.Run(dir, "pull", "--rebase"); err != nil {
		// Nhiều khả năng là rebase conflict. KHÔNG commit đè marker: abort để
		// quay lại commit của ta (an toàn, chưa push), báo và bỏ qua lần này.
		_, _ = r.Run(dir, "rebase", "--abort")
		return note(fmt.Sprintf("docs sync: pull/rebase lỗi: %v — đã abort, commit local giữ nguyên (chưa push). Bỏ qua sync lần này (fail-open)", err))
	}
	if _, err := r.Run(dir, "push"); err != nil {
		return note(fmt.Sprintf("docs sync: push lỗi: %v (fail-open, commit local giữ nguyên)", err))
	}
	return note("docs sync: pushed")
}

func note(s string) []string { return []string{s} }
