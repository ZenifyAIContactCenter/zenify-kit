package plugin

// Leg2Recommendations: repo → recommended `npx skills add` command (clean-license vendors only).
// mongodb Apache-2.0, redis MIT, vercel-labs react-best-practices (permissive).
var Leg2Recommendations = map[string][]string{
	"contact-center-hub":       {"npx skills add mongodb/agent-skills", "npx skills add redis/agent-skills"},
	"notification-hub-be":      {"npx skills add mongodb/agent-skills", "npx skills add redis/agent-skills"},
	"contact-center-be":        {"npx skills add mongodb/agent-skills", "npx skills add redis/agent-skills"},
	"chatting":                 {"npx skills add mongodb/agent-skills", "npx skills add redis/agent-skills"},
	"notification":             {"npx skills add mongodb/agent-skills", "npx skills add redis/agent-skills"},
	"change-stream-subscriber": {"npx skills add mongodb/agent-skills", "npx skills add redis/agent-skills"},
	"contact-center-web":       {"npx skills add vercel-labs/agent-skills --skill react-best-practices"},
	"notification-hub-web":     {"npx skills add vercel-labs/agent-skills --skill react-best-practices"},
}

func Leg2ForRepo(repo string) []string {
	return Leg2Recommendations[repo]
}
