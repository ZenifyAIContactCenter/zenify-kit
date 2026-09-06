// Package migrate lập kế hoạch + thực hiện gom repo vào một thư mục con (repos/).
// Thuần: mọi I/O + kiểm tra git đều inject. Fail-open.
package migrate

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
)

type Action string

const (
	Move   Action = "MOVE"
	Refuse Action = "REFUSE"
	Skip   Action = "SKIP" // đã ở trong toDir → bỏ qua (idempotent)
)

type Item struct {
	Name, From, To string
	Action         Action
	Reason         string
}

// BuildPlan phân loại mỗi repo: SKIP nếu đã ở toDir; REFUSE nếu trùng basename ở
// nhiều nơi (không gom tự động được); còn lại MOVE. KHÔNG còn gate dirty/worktree —
// v2 move repo dirty (os.Rename mang theo) và repair worktree sau move (xem Apply).
func BuildPlan(root, toDir string, repos []workspace.Repo) []Item {
	target := filepath.Join(root, toDir)
	seen := map[string]int{}
	for _, rp := range repos {
		seen[rp.Name]++
	}
	var items []Item
	for _, rp := range repos {
		it := Item{Name: rp.Name, From: rp.Path}
		switch {
		case seen[rp.Name] > 1:
			it.Action = Refuse
			it.Reason = "trùng tên repo ở nhiều nơi — không gom tự động, xử tay"
		case filepath.Dir(rp.Path) == target:
			it.Action = Skip
			it.Reason = "đã ở " + toDir + "/"
		default:
			it.To = filepath.Join(target, rp.Name)
			it.Action = Move
		}
		items = append(items, it)
	}
	return items
}

// ApplyIO gộp mọi I/O inject cho Apply (test bằng fake, thật bằng gitx+os ở CLI).
type ApplyIO struct {
	ListWT     func(repoDir string) ([]string, error)          // liệt kê worktree TRƯỚC move
	Move       func(from, to string) error                     // os.Rename
	MkdirAll   func(dir string) error                          // tạo thư mục đích
	Repair     func(repoDir, wtPath string) error               // git worktree repair <path-mới>
	Repoint    func(wtOld, wtNew, mainOld, mainNew string) error // re-point symlink node_modules
	UpdateYAML func(name, newPath string) error                 // cập nhật repos.yaml
}

// newWorktreePath ánh xạ path worktree cũ → path sau move. Worktree nội bộ (nằm dưới
// repoOld) rebase sang repoNew; worktree ngoài repo (herdr) giữ nguyên (không di chuyển).
func newWorktreePath(wtOld, repoOld, repoNew string) string {
	rel, err := filepath.Rel(repoOld, wtOld)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return wtOld // ngoài repoOld → không đổi
	}
	return filepath.Join(repoNew, rel)
}

// Apply thực hiện move-and-repair, HAI PASS cho repos.yaml (manifest ở vị trí cuối sau
// khi mọi move settle). Mỗi repo atomic: liệt-kê-worktree → move → repair+repoint từng
// worktree → rollback move-back nếu bước nào fail (repo đó Refuse, không update YAML).
// Fail-open giữa các repo. Trả về các note.
func Apply(items []Item, io ApplyIO) []string {
	var notes []string
	var moved []Item

	// Pass 1: move + repair từng repo (atomic).
	for _, it := range items {
		if it.Action != Move {
			continue
		}
		wts, err := io.ListWT(it.From)
		if err != nil {
			notes = append(notes, "bỏ qua "+it.Name+": không liệt kê được worktree: "+err.Error())
			continue
		}
		if err := io.MkdirAll(filepath.Dir(it.To)); err != nil {
			notes = append(notes, "không tạo được thư mục cho "+it.Name+": "+err.Error())
			continue
		}
		if err := io.Move(it.From, it.To); err != nil {
			notes = append(notes, "không move được "+it.Name+": "+err.Error())
			continue
		}
		failed := ""
		for _, wt := range wts {
			wtNew := newWorktreePath(wt, it.From, it.To)
			if err := io.Repair(it.To, wtNew); err != nil {
				failed = "repair worktree " + wt + ": " + err.Error()
				break
			}
			if err := io.Repoint(wt, wtNew, it.From, it.To); err != nil {
				failed = "re-point symlink " + wt + ": " + err.Error()
				break
			}
		}
		if failed != "" {
			if err := io.Move(it.To, it.From); err != nil {
				notes = append(notes, "NGHIÊM TRỌNG "+it.Name+": "+failed+" VÀ rollback fail: "+err.Error()+" — kiểm tra tay")
			} else {
				notes = append(notes, "refuse "+it.Name+": "+failed+" (đã rollback về chỗ cũ)")
			}
			continue
		}
		moved = append(moved, it)
		if len(wts) > 0 {
			notes = append(notes, "đã move "+it.Name+" + repair "+strconv.Itoa(len(wts))+" worktree")
		}
	}

	// Pass 2: mọi move/repair đã settle → cập nhật repos.yaml.
	for _, it := range moved {
		newPath := filepath.Base(filepath.Dir(it.To)) + "/" + it.Name
		if err := io.UpdateYAML(it.Name, newPath); err != nil {
			notes = append(notes, "đã move "+it.Name+" nhưng không cập nhật được repos.yaml: "+err.Error())
			continue
		}
		notes = append(notes, "đã cập nhật repos.yaml: "+it.Name+" → "+newPath)
	}
	return notes
}
