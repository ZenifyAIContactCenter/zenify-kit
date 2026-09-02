package plugin

// RepoSkills: repo core → subset skill leg-1 (theo footprint stack).
var RepoSkills = map[string][]string{
	"contact-center-hub":       {"nestjs-patterns", "mongoose-modeling", "mongo-data-safety", "service-integration"},
	"notification-hub-be":      {"nestjs-patterns", "mongoose-modeling", "mongo-data-safety", "service-integration"},
	"contact-center-be":        {"express-service-patterns", "mongoose-modeling", "mongo-data-safety", "service-integration"},
	"chatting":                 {"express-service-patterns", "mongoose-modeling", "mongo-data-safety", "service-integration"},
	"notification":             {"express-service-patterns", "mongo-data-safety", "service-integration"},
	"contact-center-web":       {"react-patterns"},
	"notification-hub-web":     {"react-patterns"},
	"change-stream-subscriber": {"mongo-data-safety", "service-integration"},
}

func SkillsForRepo(repo string) []string {
	return RepoSkills[repo]
}

// Leg2Recommendations: repo → lệnh `npx skills add` khuyến nghị (chỉ vendor license sạch).
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
