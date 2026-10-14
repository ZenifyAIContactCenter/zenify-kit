package dbread

import (
	"errors"
	"fmt"
	"strings"
)

// sqlAllowed are the statement-leading keywords a read may start with. DDL
// auto-commits and escapes a READ ONLY transaction, so an allowlist is needed.
var sqlAllowed = map[string]bool{
	"SELECT": true, "WITH": true, "SHOW": true,
	"DESCRIBE": true, "DESC": true, "EXPLAIN": true,
}

func isASCIILetter(c byte) bool { return (c|0x20) >= 'a' && (c|0x20) <= 'z' }

func isIdentChar(c byte) bool {
	return isASCIILetter(c) || (c >= '0' && c <= '9') || c == '_' || c == '$'
}

// sqlBanned are bare words that make a statement write, or dump to a file/variable.
var sqlBanned = map[string]bool{"DELETE": true, "UPDATE": true, "INSERT": true, "INTO": true}

// CheckSQL rejects anything but a single read statement. One stateful pass
// (outside / in ' / in " / in `) so quotes inside comments cannot desync it:
// comments are rejected outright, outside literals, at any position. Bare words
// outside literals/backticks are checked against a write/lock denylist.
//
// It assumes the sql_mode that WrapSQL sets (no NO_BACKSLASH_ESCAPES, no
// ANSI_QUOTES), so CheckSQL and WrapSQL must be used together.
func CheckSQL(stmt string) error {
	s := strings.TrimSpace(stmt)
	if s == "" {
		return errors.New("câu SQL rỗng") //znf:allow-lang
	}
	n := 0
	for n < len(s) && isASCIILetter(s[n]) {
		n++
	}
	if !sqlAllowed[strings.ToUpper(s[:n])] {
		return errors.New("chỉ cho phép SELECT/WITH/SHOW/DESCRIBE/DESC/EXPLAIN") //znf:allow-lang
	}
	var quote byte // 0 = outside a literal
	prev := ""     // previous bare word, upper-cased
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			switch {
			case c == '\\' && quote != '`':
				i++ // skip the escaped character
			case c == quote:
				quote = 0 // a doubled quote closes then reopens on the next char
			}
			continue
		}
		if isIdentChar(c) {
			j := i
			for j < len(s) && isIdentChar(s[j]) {
				j++
			}
			w := strings.ToUpper(s[i:j])
			if sqlBanned[w] || (w == "SHARE" && (prev == "FOR" || prev == "IN")) {
				return fmt.Errorf("từ khoá %s bị từ chối", w) //znf:allow-lang
			}
			prev = w
			i = j - 1
			continue
		}
		switch {
		case c == '\'' || c == '"' || c == '`':
			quote = c
		case c == '#' || (c == '-' && i+1 < len(s) && s[i+1] == '-') || (c == '/' && i+1 < len(s) && s[i+1] == '*'):
			return errors.New("comment SQL bị từ chối") //znf:allow-lang
		case c == ';':
			if strings.TrimSpace(s[i+1:]) != "" {
				return errors.New("chỉ cho phép một câu lệnh") //znf:allow-lang
			}
		case c == '\\':
			// mysql client commands (\!, \., \g ...) run even mid-line without --binary-mode
			return errors.New("ký tự \\ ngoài chuỗi bị từ chối") //znf:allow-lang
		}
	}
	if quote != 0 {
		return errors.New("chuỗi literal chưa đóng") //znf:allow-lang
	}
	return nil
}

// sqlMode is the fixed MySQL 8 default; it keeps backslash escapes and
// double-quoted strings, which CheckSQL's scanner relies on.
const sqlMode = "ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION"

// WrapSQL runs stmt in a READ ONLY transaction with a server-side timeout.
// SQL_SELECT_LIMIT is limit+1 so the caller can detect truncation.
func WrapSQL(stmt string, limit int) string {
	s := strings.TrimSuffix(strings.TrimSpace(stmt), ";")
	return fmt.Sprintf("START TRANSACTION READ ONLY; SET SESSION sql_mode='"+sqlMode+"'; SET SESSION MAX_EXECUTION_TIME=%d; SET SESSION SQL_SELECT_LIMIT=%d; %s; ROLLBACK;",
		QueryTimeoutMS, limit+1, s)
}
