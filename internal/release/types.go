package release

// Commit là một commit trong khoảng release, đã phân loại.
type Commit struct {
	SHA     string // short sha
	Subject string
	Type    string // feat|fix|perf|refactor|chore|other
	Merge   bool
	Branch  string // branch nguồn parse từ merge subject; "" nếu không có
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
}

// Report là toàn bộ report của một release.
type Report struct {
	N               int
	GeneratedAt     string
	Repos           []RepoReport        // repo tham gia
	NotShipped      []string            // repo theo dõi mà không có release<N>
	SharedCrossRepo map[string][]string // pattern -> repos (>=2)
	DeployOrderNote bool
}
