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
