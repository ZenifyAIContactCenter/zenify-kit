package api

import "strings"

// What role dev may read on prd (FR-14.2, FR-14.3): structure, plans and
// metadata, never record content. Shared by the broker, which enforces it,
// and the kit, which refuses early from its cached role.

// devMongoOps are the Mongo ops a dev may run on prd: structure and plans,
// never document content (FR-14.2).
var devMongoOps = map[string]bool{"collections": true, "count": true, "explain": true, "indexes": true, "stats": true}

// DevQueryAllowed reports whether a dev may send req to prd. It only narrows:
// the broker's dbread guards still run afterwards. The kit uses it too, to
// refuse locally before calling the broker.
func DevQueryAllowed(req QueryRequest) bool {
	switch {
	case req.Target != "" && req.Target != "main":
		return false
	case req.Engine == "mongo":
		return devMongoOps[req.Op]
	case req.Engine == "mysql":
		return req.Op == "tables" || (req.Op == "sql" && devSQL(req.SQL))
	}
	return false
}

// devSQL reports whether stmt, after leading whitespace and comments, starts
// with a plan or schema statement: EXPLAIN [ANALYZE] [FORMAT=x] SELECT|WITH,
// DESCRIBE/DESC <table> (or with an EXPLAIN tail), or SHOW INDEX|INDEXES|KEYS|COLUMNS|TABLES|CREATE TABLE|TABLE
// STATUS.
func devSQL(stmt string) bool {
	w := leadingWords(stmt, 6)
	at := func(i int) string {
		if i < len(w) {
			return w[i]
		}
		return ""
	}
	// explainTail checks what follows EXPLAIN at i: [ANALYZE] [FORMAT=x]
	// then SELECT|WITH. FOR CONNECTION (another session's plan) never passes.
	explainTail := func(i int) bool {
		if at(i) == "ANALYZE" {
			i++
		}
		if at(i) == "FORMAT" && at(i+1) == "=" && at(i+2) != "" {
			i += 3
		}
		return at(i) == "SELECT" || at(i) == "WITH"
	}
	switch at(0) {
	case "DESCRIBE", "DESC":
		// DESCRIBE is EXPLAIN's synonym: a statement after it is an explain
		// and gets the same rule; otherwise it describes a table.
		switch at(1) {
		case "ANALYZE", "FORMAT", "FOR", "SELECT", "WITH", "TABLE", "DELETE", "INSERT", "REPLACE", "UPDATE":
			return explainTail(1)
		}
		return true
	case "SHOW":
		switch at(1) {
		case "INDEX", "INDEXES", "KEYS", "COLUMNS", "TABLES":
			return true
		case "CREATE":
			return at(2) == "TABLE"
		case "TABLE":
			return at(2) == "STATUS"
		}
	case "EXPLAIN":
		return explainTail(1)
	}
	return false
}

// leadingWords returns up to n upper-cased leading tokens of s: identifier
// words and "=", skipping whitespace and /* */, -- and # comments. It stops
// at any other character or an unterminated comment.
func leadingWords(s string, n int) []string {
	var out []string
	for len(out) < n {
		s = strings.TrimLeft(s, " \t\r\n\f\v")
		switch {
		case strings.HasPrefix(s, "/*"):
			end := strings.Index(s[2:], "*/")
			if end < 0 {
				return out
			}
			s = s[2+end+2:]
		case strings.HasPrefix(s, "--"), strings.HasPrefix(s, "#"):
			end := strings.IndexAny(s, "\r\n")
			if end < 0 {
				return out
			}
			s = s[end:]
		case strings.HasPrefix(s, "="):
			out, s = append(out, "="), s[1:]
		default:
			j := 0
			for j < len(s) && (s[j] == '_' || s[j] >= '0' && s[j] <= '9' || (s[j]|0x20) >= 'a' && (s[j]|0x20) <= 'z') {
				j++
			}
			if j == 0 {
				return out
			}
			out, s = append(out, strings.ToUpper(s[:j])), s[j:]
		}
	}
	return out
}

// devRedis are the metadata commands a dev may send to prd: key names, types,
// TTLs and sizes, never values.
var devRedis = map[string]bool{"TTL": true, "TYPE": true, "EXISTS": true, "SCAN": true, "LLEN": true, "SCARD": true, "ZCARD": true}

// DevRedisAllowed reports whether a dev may send cmd to prd Redis.
func DevRedisAllowed(cmd string) bool {
	return devRedis[strings.ToUpper(strings.TrimSpace(cmd))]
}
