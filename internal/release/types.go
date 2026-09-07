package release

// Commit là một commit trong khoảng release, đã phân loại.
type Commit struct {
	SHA     string // short sha
	Subject string
	Type    string // feat|fix|perf|refactor|chore|other
	Merge   bool
	Branch  string // branch nguồn parse từ merge subject; "" nếu không có
	Body    string
}

// RepoReport là phần report cho một repo trong một release.
type RepoReport struct {
	Name                 string
	PrevRelease          int
	CutDate              string
	Commits              []Commit
	TypeCounts           map[string]int
	HasMigration         bool
	SharedHits           []string // glob patterns khớp
	HasTestTouch         bool
	Regression           []Commit
	RegressionUncomputed bool // true nếu so sánh với staging KHÔNG chạy được (khác với "sạch")
	Hotfixes             []Commit
	Err                  string // note fail-open; "" nếu ok
	Changes              []Change
}

// Change = một "thay đổi" (feature/fix/hotfix) gom từ nhiều commit cùng branch-slug/scope.
type Change struct {
	Title        string   // humanize từ Slug
	Slug         string   // key chuẩn-hoá (last-segment branch ∪ scope)
	Type         string   // feat|fix|hotfix|chore|other
	PRNum        string   // "" nếu không parse được
	Commits      []Commit // các commit attribute vào thay đổi này
	IsHotfix     bool
	NotOnStaging bool // có commit thuộc tập NotInStaging
	Risk         RiskMeta
}

// RiskMeta = risk-metadata kéo từ spec Brief (M6c1). SpecPath rỗng = "unknown — no spec".
type RiskMeta struct {
	SpecPath    string
	BlastRadius string
	DB          string
	Rollback    string
}

// SpecMeta = spec đã parse sẵn (path + slug tokens + 3 tag Brief) để LinkSpec khớp thuần.
type SpecMeta struct {
	Path        string
	Slug        string // token từ tên file, dùng fuzzy-match
	BlastRadius string
	DB          string
	Rollback    string
}

// Report là toàn bộ report của một release.
type Report struct {
	N               int
	GeneratedAt     string
	Repos           []RepoReport        // repo tham gia
	NotShipped      []string            // repo theo dõi mà không có release<N>
	SharedCrossRepo map[string][]string // pattern -> repos (>=2)
	DeployOrderNote bool

	ShippingRepos     []string // repo có release<N>
	TotalFeat         int
	TotalFix          int
	TotalHotfix       int
	HotfixesNotSynced int      // hotfix có commit chưa trên staging
	Migrations        []string // repo có migration
	SpecLinked        int      // số Change link được spec
	SpecTotal         int      // tổng Change (mọi type trừ chore? — xem Task 5)
}
