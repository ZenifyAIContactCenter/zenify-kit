package plugin

import "testing"

func TestSkillsForRepoCore(t *testing.T) {
	cases := map[string][]string{
		"contact-center-web": {"react-patterns"},
		"contact-center-be":  {"express-service-patterns", "mongoose-modeling", "mongo-data-safety", "service-integration"},
		"contact-center-hub": {"nestjs-patterns", "mongoose-modeling", "mongo-data-safety", "service-integration"},
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
