// Package migrate lập kế hoạch + thực hiện gom repo vào một thư mục con (repos/).
// Thuần: mọi I/O + kiểm tra git đều inject. Fail-open.
package migrate

import (
	"path/filepath"

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

// Apply thực hiện MOVE trong HAI PASS để tách vị trí manifest khỏi thứ tự move:
// pass 1 move hết mọi repo (kể cả repo chứa manifest), pass 2 mới cập nhật repos.yaml
// — nên khi updateYAML resolve đường dẫn manifest thì mọi repo đã ở vị trí cuối.
// Lỗi một repo thành note, tiếp tục repo sau (fail-open). Repo move lỗi thì KHÔNG update YAML.
func Apply(items []Item, move func(from, to string) error,
	mkdirAll func(string) error, updateYAML func(name, newPath string) error) []string {

	var notes []string
	var moved []Item // các item move thành công → mới được update YAML ở pass 2

	// Pass 1: move hết.
	for _, it := range items {
		if it.Action != Move {
			continue
		}
		if err := mkdirAll(filepath.Dir(it.To)); err != nil {
			notes = append(notes, "không tạo được thư mục cho "+it.Name+": "+err.Error())
			continue
		}
		if err := move(it.From, it.To); err != nil {
			notes = append(notes, "không move được "+it.Name+": "+err.Error())
			continue
		}
		moved = append(moved, it)
	}

	// Pass 2: mọi move đã settle → manifest ở vị trí cuối, giờ mới cập nhật repos.yaml.
	for _, it := range moved {
		newPath := filepath.Base(filepath.Dir(it.To)) + "/" + it.Name
		if err := updateYAML(it.Name, newPath); err != nil {
			notes = append(notes, "đã move "+it.Name+" nhưng không cập nhật được repos.yaml: "+err.Error())
			continue
		}
		notes = append(notes, "đã move "+it.Name+" → "+newPath)
	}
	return notes
}
