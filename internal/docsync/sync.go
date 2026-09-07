// Package docsync đồng bộ repo docs (agent-managed): commit thay đổi agent
// ghi, rồi rebase lên remote và push. Model Obsidian git-sync. Thuần: inject
// gitx.Runner.
package docsync

import (
	"fmt"
	"strconv"
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
//
// Sạch nhưng CÒN commit chưa push (đường clean-but-ahead): một lần pull/rebase
// fail transient trước đó khiến commit đã tạo bị kẹt local. Turn sau tree sạch,
// nên nếu "sạch → trả ngay" thì commit kẹt vĩnh viễn. Vì vậy khi sạch ta vẫn
// đếm ahead bằng rev-list LOCAL (không network); chỉ khi ahead>0 mới đụng mạng
// để đẩy nốt qua đúng đường rebase+push.
func Sync(r gitx.Runner, dir string) []string {
	st, err := r.Run(dir, "status", "--porcelain")
	if err != nil {
		return note(fmt.Sprintf("docs sync: status lỗi: %v (fail-open)", err))
	}
	if strings.TrimSpace(string(st)) == "" {
		// Không có gì để commit — nhưng có thể còn commit local chưa push.
		if aheadCommits(r, dir) == 0 {
			return note("docs sync: clean") // không network, không commit
		}
		return pushPending(r, dir) // commit kẹt từ lần trước → đẩy nốt
	}
	if _, err := r.Run(dir, "add", "-A"); err != nil {
		return note(fmt.Sprintf("docs sync: add lỗi: %v (fail-open)", err))
	}
	msg := "chore(docs): sync " + time.Now().UTC().Format("2006-01-02T15:04:05Z")
	if _, err := r.Run(dir, "commit", "-m", msg); err != nil {
		return note(fmt.Sprintf("docs sync: commit lỗi: %v (fail-open)", err))
	}
	return pushPending(r, dir)
}

// aheadCommits đếm commit local chưa có trên upstream, THUẦN LOCAL (không
// network — dùng ref remote-tracking sẵn có). Không xác định được (chưa set
// upstream, output không parse) → 0, để giữ fast-path "sạch = không đụng mạng".
func aheadCommits(r gitx.Runner, dir string) int {
	out, err := r.Run(dir, "rev-list", "--count", "@{upstream}..HEAD")
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return n
}

// pushPending rebase commit local lên remote rồi push. Conflict → abort, giữ
// commit local nguyên vẹn (chưa push), fail-open. Dùng chung cho đường dirty
// (vừa commit) và đường clean-but-ahead (commit kẹt từ lần trước).
func pushPending(r gitx.Runner, dir string) []string {
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
