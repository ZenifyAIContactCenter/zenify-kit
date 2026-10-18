package obssvc

import (
	"regexp"
	"strings"
)

// secretKeys are the keys whose value Mask hides in key=value, key: value and
// "key":"value" forms.
const secretKeys = `password|passwd|pwd|secret|token|access_token|refresh_token|api_key|apikey|x-api-key|authorization`

var (
	// reCred: the user is optional and the password runs to the last @ of the authority.
	reCred   = regexp.MustCompile(`(://)[^/\s:@]*:[^/\s]*@`)
	reBearer = regexp.MustCompile(`(?i)bearer\s+\S+`)
	reBasic  = regexp.MustCompile(`(?i)\bbasic\s+[A-Za-z0-9+/]{8,}={0,2}`)
	// reKVQuoted masks a quoted value, reKV a bare one; the key must stand alone.
	reKVQuoted = regexp.MustCompile(`(?i)(^|[^\w-])("?(?:` + secretKeys + `)"?[ \t]*[:=][ \t]*")[^"]*"`)
	reKV       = regexp.MustCompile(`(?i)(^|[^\w-])((?:` + secretKeys + `)[ \t]*[:=][ \t]*)([^\s"'&,;]+)`)
	reJWT      = regexp.MustCompile(`eyJ[\w-]+\.[\w-]+\.[\w-]+`)
	reEmail    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	reIPv4     = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	rePhone    = regexp.MustCompile(`\+?\d(?:[ .-]?\d){8,10}`)

	reKVSingle = regexp.MustCompile(`(?i)(^|[^\w-])('?(?:` + secretKeys + `)'?[ \t]*[:=][ \t]*')[^']*'`)
	// reAuth: an authorization value runs to the end of the field, whatever its scheme.
	reAuth = regexp.MustCompile(`(?i)(^|[^\w-])(authorization[ \t]*[:=][ \t]*)([^\s"'][^\r\n"'&,;]*)`)
)

// Mask replaces credentials, tokens, emails, IPs and phone numbers with
// labels, in a fixed order. It only catches the patterns above.
func Mask(s string) string {
	s = reCred.ReplaceAllString(s, "${1}<cred>@")
	s = reBearer.ReplaceAllString(s, "Bearer <token>")
	s = reBasic.ReplaceAllString(s, "Basic <token>")
	s = reJWT.ReplaceAllString(s, "<jwt>")
	s = reKVQuoted.ReplaceAllString(s, `${1}${2}<secret>"`)
	s = reKVSingle.ReplaceAllString(s, `${1}${2}<secret>'`)
	s = reAuth.ReplaceAllStringFunc(s, maskAuth)
	s = reKV.ReplaceAllStringFunc(s, maskKV)
	s = reEmail.ReplaceAllString(s, "<email>")
	s = reIPv4.ReplaceAllString(s, "<ip>")
	return maskPhone(s)
}

// maskKV hides the value of one reKV match, keeping a Bearer or Basic scheme
// word that the token masks already handled.
func maskKV(m string) string {
	g := reKV.FindStringSubmatch(m)
	if v := strings.ToLower(g[3]); v == "bearer" || v == "basic" {
		return m
	}
	return g[1] + g[2] + "<secret>"
}

// maskAuth hides an authorization value, keeping only a Bearer or Basic token
// the token masks already replaced.
func maskAuth(m string) string {
	g := reAuth.FindStringSubmatch(m)
	if strings.HasPrefix(g[3], "Bearer <token>") || strings.HasPrefix(g[3], "Basic <token>") {
		return m
	}
	return g[1] + g[2] + "<secret>"
}

// maskPhone applies rePhone, accepting a match only when it uses a single
// kind of separator and stands alone (RE2 has no lookbehind).
func maskPhone(s string) string {
	var out []byte
	last := 0
	for pos := 0; pos < len(s); {
		loc := rePhone.FindStringIndex(s[pos:])
		if loc == nil {
			break
		}
		start, end := pos+loc[0], pos+loc[1]
		if e := phoneEnd(s, start, end); e > 0 {
			out = append(out, s[last:start]...)
			out = append(out, "<phone>"...)
			last = e
			pos = e
		} else {
			pos = start + 1 // a valid phone may overlap the rejected match's tail
		}
	}
	return string(append(out, s[last:]...))
}

// phoneEnd returns the end of the longest valid phone starting at start and
// no longer than end, trimming trailing digits down to 9 digits; 0 if none.
func phoneEnd(s string, start, end int) int {
	for e := end; e > start; e-- {
		if s[e-1] < '0' || s[e-1] > '9' {
			continue
		}
		c := s[start:e]
		if digits(c) < 9 {
			return 0
		}
		if singleSep(c) && standalone(s, start, e) {
			return e
		}
	}
	return 0
}

func digits(m string) int {
	n := 0
	for i := 0; i < len(m); i++ {
		if m[i] >= '0' && m[i] <= '9' {
			n++
		}
	}
	return n
}

func singleSep(m string) bool {
	var sep byte
	for i := 0; i < len(m); i++ {
		c := m[i]
		if c == ' ' || c == '.' || c == '-' {
			if sep != 0 && sep != c {
				return false
			}
			sep = c
		}
	}
	return true
}

func standalone(s string, start, end int) bool {
	return (start == 0 || !wordish(s, start-1, -1)) && (end == len(s) || !wordish(s, end, 1))
}

// wordish reports whether s[i] joins the phone to its neighbour: a letter or
// digit, or a ':' or '/' whose next character away from the phone (dir) is a
// digit, as in 12:34 or 2026/10/18.
func wordish(s string, i, dir int) bool {
	c := s[i]
	if c == ':' || c == '/' {
		j := i + dir
		return j >= 0 && j < len(s) && isDigit(s[j])
	}
	return isDigit(c) || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
