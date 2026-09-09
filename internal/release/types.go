package release

// Commit is one commit within a release range, already classified.
type Commit struct {
	SHA      string // short sha
	Subject  string
	Author   string // git author name (%an)
	Type     string // feat|fix|perf|refactor|chore|other
	Merge    bool
	Branch   string // source branch parsed from the merge subject; "" if none
	Body     string
	PRBranch string // the branch of the PR this commit belongs to (set by RangeCommitsGrouped); "" if a standalone commit not in a PR
}

// RepoReport is the report section for one repo in one release.
type RepoReport struct {
	Name                 string
	PrevRelease          int
	CutDate              string
	Commits              []Commit
	TypeCounts           map[string]int
	HasMigration         bool
	SharedHits           []string // matched glob patterns
	HasTestTouch         bool
	Regression           []Commit
	RegressionUncomputed bool // true if the comparison against staging could NOT run (different from "clean")
	Hotfixes             []Commit
	Err                  string // fail-open note; "" if ok
	Changes              []Change
}

// Change = one "change" (feature/fix/hotfix) grouped from several commits sharing a branch-slug/scope.
type Change struct {
	Title        string   // humanized from Slug
	Slug         string   // normalized key (last-segment branch ∪ scope)
	Type         string   // feat|fix|hotfix|chore|other
	PRNum        string   // "" if it couldn't be parsed
	Commits      []Commit // the commits attributed to this change
	Authors      []string // the devs who worked on it (distinct, first-seen order)
	Desc         string   // representative description (first non-merge commit subject, type(scope): prefix stripped)
	IsHotfix     bool
	NotOnStaging bool // has a commit in the NotInStaging set
	Risk         RiskMeta
}

// RiskMeta = risk metadata pulled from the spec Brief (M6c1). An empty SpecPath = "unknown — no spec".
type RiskMeta struct {
	SpecPath    string
	BlastRadius string
	DB          string
	Rollback    string
	Note        string // one-line description (prose, usually Vietnamese) from the note-commit's _Release-Note trailer; "" when the risk came from a spec, not a note
}

// SpecMeta = a pre-parsed spec (path + slug tokens + the 3 Brief tags) so LinkSpec can match purely.
type SpecMeta struct {
	Path        string
	Slug        string // token from the file name, used for fuzzy-matching
	BlastRadius string
	DB          string
	Rollback    string
	Supersedes  string // raw slug from the _Supersedes: Brief tag; "" if none (the only non-derivable lifecycle input)
}

// LinkTier records which rule linked a Change to a spec. Ordered by strength (higher wins) so a
// caller can take the max across several Changes: a note beats a Spec: trailer, which beats an
// exact slug, which beats a fuzzy slug. Mirrors the match order in LinkSpecTier.
type LinkTier int

const (
	TierNone      LinkTier = iota // no link
	TierSlugFuzzy                 // slug substring match
	TierSlugExact                 // slug equality
	TierTrailer                   // Spec: <path> commit trailer
	TierNote                      // release note-commit linked by slug
)

// Report is the whole report for one release.
type Report struct {
	N               int
	GeneratedAt     string
	Repos           []RepoReport        // repos included
	NotShipped      []string            // tracked repos with no release<N>
	SharedCrossRepo map[string][]string // pattern -> repos (>=2)
	DeployOrderNote bool

	ShippingRepos     []string // repos with release<N>
	TotalFeat         int
	TotalFix          int
	TotalHotfix       int
	HotfixesNotSynced int      // hotfixes with a commit not yet on staging
	Migrations        []string // repos with a migration
	SpecLinked        int      // number of Changes linked to a spec
	SpecTotal         int      // total Changes (every type except chore? — see Task 5)

	Unreleased bool // true when the report is the "release still forming" view (range release<latest>..staging)
}
