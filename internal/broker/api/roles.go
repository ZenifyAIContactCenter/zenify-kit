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
	case req.Engine == "mongo" && req.Op == "count":
		// A filtered count answers yes/no about record content ({"email":
		// {"$regex":"^a"}}), so dev gets the unfiltered estimate only.
		return strings.TrimSpace(req.Filter) == ""
	case req.Engine == "mongo":
		return devMongoOps[req.Op]
	case req.Engine == "mysql":
		return req.Op == "tables" || (req.Op == "sql" && devSQL(req.SQL))
	}
	return false
}

// devSQL reports whether stmt is a plan or schema statement a dev may run on
// prd, judged on the whole statement, not just its head:
//   - EXPLAIN [FORMAT=x] SELECT … with exactly one SELECT and no subquery.
//     ANALYZE executes, and the optimizer evaluates constant subqueries while
//     planning, so either would read rows.
//   - DESCRIBE/DESC <table> and SHOW INDEX|INDEXES|KEYS|COLUMNS|TABLES|CREATE
//     TABLE|TABLE STATUS: keywords, identifiers and LIKE '<literal>' only. A
//     WHERE, a "(" or a SELECT there is a boolean or error channel into rows.
//   - DESCRIBE/DESC followed by a statement keyword is EXPLAIN's synonym and
//     gets the EXPLAIN rule.
func devSQL(stmt string) bool {
	t, ok := sqlTokens(stmt)
	if !ok || len(t) == 0 {
		return false
	}
	at := func(i int) string {
		if i < len(t) {
			return t[i]
		}
		return ""
	}
	switch t[0] {
	case "EXPLAIN":
		return explainBody(t[1:])
	case "DESCRIBE", "DESC":
		switch at(1) {
		case "ANALYZE", "FORMAT", "FOR", "SELECT", "WITH", "TABLE", "DELETE", "INSERT", "REPLACE", "UPDATE":
			return explainBody(t[1:])
		}
		return isIdent(at(1)) && schemaOnly(t)
	case "SHOW":
		switch at(1) {
		case "INDEX", "INDEXES", "KEYS", "COLUMNS", "TABLES":
			return schemaOnly(t)
		case "CREATE":
			return at(2) == "TABLE" && schemaOnly(t)
		case "TABLE":
			return at(2) == "STATUS" && schemaOnly(t)
		}
	}
	return false
}

// explainBody checks what follows EXPLAIN: an optional FORMAT=x, then one
// SELECT with no second SELECT and no "(" opening a subquery. ANALYZE, FOR
// CONNECTION, WITH and every other statement fail.
func explainBody(t []string) bool {
	if len(t) >= 3 && t[0] == "FORMAT" && t[1] == "=" && isIdent(t[2]) {
		t = t[3:]
	}
	if len(t) == 0 || t[0] != "SELECT" {
		return false
	}
	for i := 1; i < len(t); i++ {
		if t[i] == "SELECT" || t[i] == "ANALYZE" {
			return false
		}
		if t[i] == "(" && i+1 < len(t) && subqueryStart[t[i+1]] {
			return false
		}
	}
	return true
}

// subqueryStart are the words that open a subquery after "(".
var subqueryStart = map[string]bool{"SELECT": true, "WITH": true, "TABLE": true, "VALUES": true}

// schemaOnly reports whether a SHOW/DESCRIBE statement holds only keywords,
// identifiers, "." and quoted literals (LIKE 'x'), with no WHERE or SELECT.
func schemaOnly(t []string) bool {
	for _, w := range t {
		switch {
		case w == "WHERE" || w == "SELECT":
			return false
		case w == "." || isIdent(w) || w[0] == '\'' || w[0] == '"':
		default:
			return false
		}
	}
	return true
}

// isIdent reports whether tok is a bare word or a backticked identifier.
func isIdent(tok string) bool {
	return tok != "" && (tok[0] == '`' || isWordChar(tok[0]))
}

func isWordChar(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || (c|0x20) >= 'a' && (c|0x20) <= 'z'
}

// sqlTokens splits stmt into upper-cased bare words, quoted literals and
// backticked identifiers (kept verbatim with their quotes) and single
// punctuation characters. Leading whitespace and comments are skipped, so the
// dbread guard, not this gate, answers a comment-prefixed statement; a
// comment after the first token, an unterminated literal or a non-trailing
// ";" makes ok false. A trailing ";" is dropped.
func sqlTokens(s string) (out []string, ok bool) {
	for {
		s = strings.TrimLeft(s, " \t\r\n\f\v")
		switch {
		case s == "":
			return out, true
		case strings.HasPrefix(s, "/*"), strings.HasPrefix(s, "--"), strings.HasPrefix(s, "#"):
			if len(out) > 0 {
				return nil, false
			}
			if strings.HasPrefix(s, "/*") {
				end := strings.Index(s[2:], "*/")
				if end < 0 {
					return nil, false
				}
				s = s[2+end+2:]
				continue
			}
			end := strings.IndexAny(s, "\r\n")
			if end < 0 {
				return nil, false
			}
			s = s[end:]
		case s[0] == '\'' || s[0] == '"' || s[0] == '`':
			q, j := s[0], 1
			for ; j < len(s); j++ {
				if s[j] == '\\' && q != '`' {
					j++
				} else if s[j] == q {
					break
				}
			}
			if j >= len(s) {
				return nil, false
			}
			out, s = append(out, s[:j+1]), s[j+1:]
		case s[0] == ';':
			if strings.TrimSpace(s[1:]) != "" {
				return nil, false
			}
			return out, true
		case isWordChar(s[0]):
			j := 0
			for j < len(s) && isWordChar(s[j]) {
				j++
			}
			out, s = append(out, strings.ToUpper(s[:j])), s[j:]
		default:
			out, s = append(out, s[:1]), s[1:]
		}
	}
}

// devRedis are the metadata commands a dev may send to prd: key names, types,
// TTLs and sizes, never values.
var devRedis = map[string]bool{"TTL": true, "TYPE": true, "EXISTS": true, "SCAN": true, "LLEN": true, "SCARD": true, "ZCARD": true}

// DevRedisAllowed reports whether a dev may send cmd to prd Redis.
func DevRedisAllowed(cmd string) bool {
	return devRedis[strings.ToUpper(strings.TrimSpace(cmd))]
}
