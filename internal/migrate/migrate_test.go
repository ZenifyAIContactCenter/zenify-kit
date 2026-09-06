package migrate

import (
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
)

func TestBuildPlanClassifies(t *testing.T) {
	root := "/ws"
	repos := []workspace.Repo{
		{Name: "alpha", Path: "/ws/alpha"},     // Move
		{Name: "beta", Path: "/ws/repos/beta"}, // Skip — đã ở repos/
		{Name: "dup", Path: "/ws/dup"},         // Refuse — trùng basename
		{Name: "dup", Path: "/ws/repos/dup"},   // Refuse — trùng basename
	}
	items := BuildPlan(root, "repos", repos)

	by := map[string]Item{}
	for _, it := range items {
		by[it.Name+"|"+it.From] = it
	}
	if it := by["alpha|/ws/alpha"]; it.Action != Move || it.To != "/ws/repos/alpha" {
		t.Fatalf("alpha: %+v", it)
	}
	if it := by["beta|/ws/repos/beta"]; it.Action != Skip {
		t.Fatalf("beta muốn Skip: %+v", it)
	}
	for _, k := range []string{"dup|/ws/dup", "dup|/ws/repos/dup"} {
		if it := by[k]; it.Action != Refuse {
			t.Fatalf("%s muốn Refuse (trùng basename): %+v", k, it)
		}
	}
}

func TestApplyMovesOnlyMove(t *testing.T) {
	items := []Item{
		{Name: "clean", From: "/ws/clean", To: "/ws/repos/clean", Action: Move},
		{Name: "dirty", From: "/ws/dirty", Action: Refuse, Reason: "dirty"},
	}
	moved := map[string]string{}
	yamls := map[string]string{}
	notes := Apply(items,
		func(from, to string) error { moved[from] = to; return nil },
		func(string) error { return nil },
		func(name, newPath string) error { yamls[name] = newPath; return nil },
	)
	if len(moved) != 1 || moved["/ws/clean"] != "/ws/repos/clean" {
		t.Fatalf("chỉ MOVE clean: %+v", moved)
	}
	if yamls["clean"] != "repos/clean" {
		t.Errorf("phải cập nhật repos.yaml path clean→repos/clean: %+v", yamls)
	}
	if len(notes) == 0 {
		t.Error("Apply phải trả note")
	}
}
