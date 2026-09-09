package speclife

import (
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/release"
)

// Contract is the registry row for one spec: the repos it says it can break (_Blast-radius:) and
// the DB surface it touches (_DB:). Derived purely from the spec Brief tags — no validation.
type Contract struct {
	SpecPath    string `json:"path"`
	Repo        string `json:"repo"`
	BlastRadius string `json:"blast_radius"`
	DB          string `json:"db"`
}

// BuildContracts returns one Contract per spec carrying a _Blast-radius: or _DB: tag. Specs with
// neither are skipped and counted (never a crash — a spec missing Brief tags is common).
func BuildContracts(specs []release.SpecMeta) (contracts []Contract, skipped int) {
	for _, s := range specs {
		if strings.TrimSpace(s.BlastRadius) == "" && strings.TrimSpace(s.DB) == "" {
			skipped++
			continue
		}
		contracts = append(contracts, Contract{
			SpecPath:    s.Path,
			Repo:        RepoOf(s.Path),
			BlastRadius: s.BlastRadius,
			DB:          s.DB,
		})
	}
	return contracts, skipped
}

// FilterByRepo keeps contracts whose BlastRadius names repo as a token.
func FilterByRepo(cs []Contract, repo string) []Contract {
	var out []Contract
	for _, c := range cs {
		if containsToken(c.BlastRadius, repo) {
			out = append(out, c)
		}
	}
	return out
}

// FilterByCollection keeps contracts whose DB field names coll as a token.
func FilterByCollection(cs []Contract, coll string) []Contract {
	var out []Contract
	for _, c := range cs {
		if containsToken(c.DB, coll) {
			out = append(out, c)
		}
	}
	return out
}

// containsToken reports whether want appears as a whole token in field. Tokens are split on
// whitespace and commas (so "contact-center-be, chatting" → two tokens); matching is
// case-insensitive. A hyphenated name like "zenify-kit" is one token.
func containsToken(field, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	for _, tok := range strings.FieldsFunc(field, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t'
	}) {
		if strings.ToLower(strings.TrimSpace(tok)) == want {
			return true
		}
	}
	return false
}
