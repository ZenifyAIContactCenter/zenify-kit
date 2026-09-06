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

// BuildPlan phân loại mỗi repo. REFUSE nếu dirty hoặc có worktree. SKIP nếu đã ở toDir.
func BuildPlan(root, toDir string, repos []workspace.Repo,
	dirty func(dir string) (bool, error), hasWT func(dir string) (bool, error)) []Item {

	target := filepath.Join(root, toDir)
	var items []Item
	for _, rp := range repos {
		it := Item{Name: rp.Name, From: rp.Path}
		if filepath.Dir(rp.Path) == target {
			it.Action = Skip
			it.Reason = "đã ở " + toDir + "/"
			items = append(items, it)
			continue
		}
		if d, err := dirty(rp.Path); err != nil || d {
			it.Action = Refuse
			it.Reason = "main checkout dirty — commit trước"
			items = append(items, it)
			continue
		}
		if w, err := hasWT(rp.Path); err != nil || w {
			it.Action = Refuse
			it.Reason = "còn worktree — `wt sweep`/merge sạch trước"
			items = append(items, it)
			continue
		}
		it.To = filepath.Join(target, rp.Name)
		it.Action = Move
		items = append(items, it)
	}
	return items
}

// Apply thực hiện MOVE: mkdir toDir, move từng repo, cập nhật repos.yaml path.
// Lỗi một repo thành note, tiếp tục repo sau (fail-open).
func Apply(items []Item, move func(from, to string) error,
	mkdirAll func(string) error, updateYAML func(name, newPath string) error) []string {

	var notes []string
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
		newPath := filepath.Base(filepath.Dir(it.To)) + "/" + it.Name
		if err := updateYAML(it.Name, newPath); err != nil {
			notes = append(notes, "đã move "+it.Name+" nhưng không cập nhật được repos.yaml: "+err.Error())
			continue
		}
		notes = append(notes, "đã move "+it.Name+" → "+newPath)
	}
	return notes
}
