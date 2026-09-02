package plugin

import (
	"strings"
	"testing"
)

func TestSkillsForRepoCore(t *testing.T) {
	cases := map[string][]string{
		"contact-center-web": {"react-patterns"},
		"contact-center-be":  {"express-service-patterns", "mongoose-modeling", "mongo-data-safety", "sql-data-safety", "service-integration"},
		"contact-center-hub": {"nestjs-patterns", "mongoose-modeling", "mongo-data-safety", "sql-data-safety", "service-integration"},
	}
	for repo, want := range cases {
		got := SkillsForRepo(repo)
		if len(got) != len(want) {
			t.Fatalf("%s: got %v want %v", repo, got, want)
		}
	}
	// tên skill phải tồn tại trong CodingSkills()
	valid := map[string]bool{}
	for _, s := range CodingSkills() {
		valid[s] = true
	}
	for repo := range cases {
		for _, s := range SkillsForRepo(repo) {
			if !valid[s] {
				t.Errorf("%s map tới skill không tồn tại: %s", repo, s)
			}
		}
	}
}

func TestLeg2ForRepo(t *testing.T) {
	for repo := range RepoSkills {
		if len(Leg2ForRepo(repo)) == 0 {
			t.Errorf("repo core %s không có khuyến nghị leg-2", repo)
		}
	}
	web := Leg2ForRepo("contact-center-web")
	joined := strings.Join(web, "\n")
	if !strings.Contains(joined, "vercel-labs/agent-skills") {
		t.Errorf("web thiếu vercel react-best-practices: %v", web)
	}
	be := strings.Join(Leg2ForRepo("contact-center-be"), "\n")
	if !strings.Contains(be, "mongodb/agent-skills") || !strings.Contains(be, "redis/agent-skills") {
		t.Errorf("be thiếu mongodb/redis official: %s", be)
	}
}
