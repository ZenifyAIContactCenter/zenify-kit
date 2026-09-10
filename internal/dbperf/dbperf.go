// Package dbperf statically scans a diff for MongoDB/SQL query anti-patterns
// and classifies each into a two-tier severity. Pure and deterministic; all
// I/O (git, files, DB) lives in the cli wrapper and the explain-plan skill.
// Every scan key is a query-language convention (.find(, $in, COLLSCAN), never
// a project-language label — the package stays project-agnostic.
package dbperf

type Tier string

const (
	Blocking Tier = "BLOCKING"
	Advisory Tier = "ADVISORY"
	Waived   Tier = "WAIVED"
)

// Finding is one detected anti-pattern at a diff line.
type Finding struct {
	Tier       Tier   `json:"tier"`
	Signal     string `json:"signal"` // skip-deep | unbounded-list | missing-projection | regex-unanchored | in-large | negation | count-filter | missing-tenant-filter | unclassified-collection | collscan | scan-ratio | sort-stage | lookup-collscan
	File       string `json:"file"`
	Line       int    `json:"line"`
	Collection string `json:"collection,omitempty"`
	Hint       string `json:"hint"`
}

// Result is the full static scan output. Dynamic findings (collscan, scan-ratio,
// sort-stage, lookup-collscan) are appended later by the explain-plan skill.
type Result struct {
	Findings       []Finding `json:"findings"`
	SitesScanned   int       `json:"sites_scanned"`
	DynamicSkipped bool      `json:"dynamic_skipped"`
}
