package dbperf

import (
	"regexp"
	"strconv"
)

var (
	callSiteRe        = regexp.MustCompile(`\.(find|findOne|aggregate|updateMany|updateOne|deleteMany|countDocuments|distinct)\(`)
	skipRe            = regexp.MustCompile(`\.skip\(\s*(\d+)\s*\)`)
	sortRe            = regexp.MustCompile(`\.sort\(`)
	limitRe           = regexp.MustCompile(`\.limit\(`)
	regexIRe          = regexp.MustCompile(`\$options\s*:\s*['"]i['"]|/[^/\n]+/[a-z]*i`)
	regexUnanchoredRe = regexp.MustCompile(`\$regex\s*:\s*['"][^^]`) // value not starting with ^
	negationRe        = regexp.MustCompile(`\$ne\b|\$nin\b`)
	inRe              = regexp.MustCompile(`\$in\s*:\s*\[([^\]]*)\]`)
	countRe           = regexp.MustCompile(`\.countDocuments\(\s*\{[^}]`)
	projFindRe        = regexp.MustCompile(`\.find\(\s*\{[^{}]*\}\s*\)`) // .find(filter) with no 2nd arg; [^{}] stops at filter's close so a projection arg does not match

	collRe        = regexp.MustCompile(`(?:\.collection|getCollection)\(\s*['"](\w+)['"]\s*\)`)
	literalFindRe = regexp.MustCompile(`\.(?:find|findOne|updateMany|updateOne|deleteMany|countDocuments)\(\s*\{([^}]*)\}`)
	tenantKeyRe   = regexp.MustCompile(`tenant_?[iI]d|tenantId`)
)

// ScanStatic scans added lines for query call-sites and text-detectable
// anti-patterns, classifying each into a two-tier severity.
func ScanStatic(added []AddedLine, cfg Config) Result {
	var r Result
	for _, a := range added {
		if !callSiteRe.MatchString(a.Text) {
			continue
		}
		r.SitesScanned++
		add := func(tier Tier, sig, coll, hint string) {
			r.Findings = append(r.Findings, Finding{Tier: tier, Signal: sig, File: a.File, Line: a.Line, Collection: coll, Hint: hint})
		}
		classifyTenant(a, cfg, add)
		// static BLOCKING signals
		if m := skipRe.FindStringSubmatch(a.Text); m != nil {
			if n, _ := strconv.Atoi(m[1]); n > cfg.SkipLarge {
				add(Blocking, "skip-deep", "", "deep pagination: dùng keyset (_id > lastSeen) thay cho skip lớn") //znf:allow-lang
			}
		}
		if sortRe.MatchString(a.Text) && !limitRe.MatchString(a.Text) {
			add(Blocking, "unbounded-list", "", "list query thiếu .limit(): giới hạn số kết quả trả về") //znf:allow-lang
		}
		// static ADVISORY signals
		if regexUnanchoredRe.MatchString(a.Text) || regexIRe.MatchString(a.Text) {
			add(Advisory, "regex-unanchored", "", "$regex không anchor ^ hoặc cờ i: không dùng được index range") //znf:allow-lang
		}
		if negationRe.MatchString(a.Text) {
			add(Advisory, "negation", "", "$ne/$nin top-level quét gần trọn index") //znf:allow-lang
		}
		if m := inRe.FindStringSubmatch(a.Text); m != nil {
			if countCommas(m[1]) >= 20 {
				add(Advisory, "in-large", "", "$in mảng lớn: nhiều index seek") //znf:allow-lang
			}
		}
		if countRe.MatchString(a.Text) {
			add(Advisory, "count-filter", "", "countDocuments có filter trên collection lớn có thể COLLSCAN") //znf:allow-lang
		}
		if projFindRe.MatchString(a.Text) {
			add(Advisory, "missing-projection", "", "find không projection: kéo cả document") //znf:allow-lang
		}
	}
	return r
}

// classifyTenant runs the FR-04/FR-08 collection classification for one site.
func classifyTenant(a AddedLine, cfg Config, add func(Tier, string, string, string)) {
	m := collRe.FindStringSubmatch(a.Text)
	if m == nil {
		return // no raw-driver collection literal → cannot classify statically
	}
	name := m[1]
	switch {
	case inList(name, cfg.Global):
		return // classified, no tenant obligation
	case !inList(name, cfg.TenantScoped):
		add(Blocking, "unclassified-collection", name,
			"collection chưa phân loại — thêm vào tenant_scoped hoặc global list ở knowledge store") //znf:allow-lang
		return
	}
	// tenant-scoped: check the filter literal for a tenant key
	if lm := literalFindRe.FindStringSubmatch(a.Text); lm != nil {
		if !tenantKeyRe.MatchString(lm[1]) {
			add(Blocking, "missing-tenant-filter", name,
				"query raw literal trên collection tenant-scoped thiếu tenant filter (leak + perf)") //znf:allow-lang
		}
		return
	}
	// dynamic filter builder: cannot verify statically
	add(Advisory, "missing-tenant-filter", name,
		"không xác minh được tenant scope ở đây (filter động) — kiểm tay") //znf:allow-lang
}

func inList(name string, list []string) bool {
	for _, x := range list {
		if x == name {
			return true
		}
	}
	return false
}

func countCommas(s string) int {
	n := 0
	for _, c := range s {
		if c == ',' {
			n++
		}
	}
	return n + 1 // element count = commas + 1
}
