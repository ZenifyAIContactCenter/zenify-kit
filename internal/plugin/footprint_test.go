package plugin

import (
	"strings"
	"testing"
)

func TestLeg2ForRepo(t *testing.T) {
	for repo := range Leg2Recommendations {
		if len(Leg2ForRepo(repo)) == 0 {
			t.Errorf("repo core %s has no leg-2 recommendation", repo)
		}
	}
	web := Leg2ForRepo("contact-center-web")
	joined := strings.Join(web, "\n")
	if !strings.Contains(joined, "vercel-labs/agent-skills") {
		t.Errorf("web missing vercel react-best-practices: %v", web)
	}
	be := strings.Join(Leg2ForRepo("contact-center-be"), "\n")
	if !strings.Contains(be, "mongodb/agent-skills") || !strings.Contains(be, "redis/agent-skills") {
		t.Errorf("be missing mongodb/redis official: %s", be)
	}
}
