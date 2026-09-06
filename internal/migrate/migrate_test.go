package migrate

import (
	"path/filepath"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
)

func TestBuildPlanClassifies(t *testing.T) {
	root := "/ws"
	repos := []workspace.Repo{
		{Name: "clean", Path: "/ws/clean"},
		{Name: "dirty", Path: "/ws/dirty"},
		{Name: "haswt", Path: "/ws/haswt"},
		{Name: "already", Path: "/ws/repos/already"},
	}
	dirty := func(dir string) (bool, error) { return dir == "/ws/dirty", nil }
	hasWT := func(dir string) (bool, error) { return dir == "/ws/haswt", nil }

	items := BuildPlan(root, "repos", repos, dirty, hasWT)
	by := map[string]Item{}
	for _, it := range items {
		by[it.Name] = it
	}
	if by["clean"].Action != Move || by["clean"].To != filepath.Join(root, "repos", "clean") {
		t.Errorf("clean phải MOVE: %+v", by["clean"])
	}
	if by["dirty"].Action != Refuse || by["dirty"].Reason == "" {
		t.Errorf("dirty phải REFUSE kèm reason: %+v", by["dirty"])
	}
	if by["haswt"].Action != Refuse {
		t.Errorf("haswt phải REFUSE: %+v", by["haswt"])
	}
	if by["already"].Action != Skip {
		t.Errorf("already trong repos/ phải SKIP: %+v", by["already"])
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
