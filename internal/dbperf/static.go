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
	projFindRe        = regexp.MustCompile(`\.find\(\s*\{[^)]*\}\s*\)`) // .find(filter) with no 2nd arg
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
		add := func(tier Tier, sig, hint string) {
			r.Findings = append(r.Findings, Finding{Tier: tier, Signal: sig, File: a.File, Line: a.Line, Hint: hint})
		}
		// TĨNH-BLOCKING
		if m := skipRe.FindStringSubmatch(a.Text); m != nil {
			if n, _ := strconv.Atoi(m[1]); n > cfg.SkipLarge {
				add(Blocking, "skip-deep", "deep pagination: dùng keyset (_id > lastSeen) thay cho skip lớn") //znf:allow-lang
			}
		}
		if sortRe.MatchString(a.Text) && !limitRe.MatchString(a.Text) {
			add(Blocking, "unbounded-list", "list query thiếu .limit(): giới hạn số kết quả trả về") //znf:allow-lang
		}
		// TĨNH-ADVISORY
		if regexUnanchoredRe.MatchString(a.Text) || regexIRe.MatchString(a.Text) {
			add(Advisory, "regex-unanchored", "$regex không anchor ^ hoặc cờ i: không dùng được index range") //znf:allow-lang
		}
		if negationRe.MatchString(a.Text) {
			add(Advisory, "negation", "$ne/$nin top-level quét gần trọn index") //znf:allow-lang
		}
		if m := inRe.FindStringSubmatch(a.Text); m != nil {
			if countCommas(m[1]) >= 20 {
				add(Advisory, "in-large", "$in mảng lớn: nhiều index seek") //znf:allow-lang
			}
		}
		if countRe.MatchString(a.Text) {
			add(Advisory, "count-filter", "countDocuments có filter trên collection lớn có thể COLLSCAN") //znf:allow-lang
		}
		if projFindRe.MatchString(a.Text) {
			add(Advisory, "missing-projection", "find không projection: kéo cả document") //znf:allow-lang
		}
	}
	return r
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
