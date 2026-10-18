package obssvc

import "regexp"

var (
	reCred   = regexp.MustCompile(`(://)[^/\s:@]+:[^/\s@]*@`)
	reBearer = regexp.MustCompile(`Bearer\s+\S+`)
	reJWT    = regexp.MustCompile(`eyJ[\w-]+\.[\w-]+\.[\w-]+`)
	reEmail  = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	reIPv4   = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	rePhone  = regexp.MustCompile(`\+?\d(?:[ .-]?\d){8,10}`)
)

// Mask replaces credentials, tokens, emails, IPs and phone numbers with
// labels, in a fixed order. It only catches the patterns above.
func Mask(s string) string {
	s = reCred.ReplaceAllString(s, "${1}<cred>@")
	s = reBearer.ReplaceAllString(s, "Bearer <token>")
	s = reJWT.ReplaceAllString(s, "<jwt>")
	s = reEmail.ReplaceAllString(s, "<email>")
	s = reIPv4.ReplaceAllString(s, "<ip>")
	return maskPhone(s)
}

// maskPhone applies rePhone, accepting a match only when it uses a single
// kind of separator and stands alone (RE2 has no lookbehind).
func maskPhone(s string) string {
	idx := rePhone.FindAllStringIndex(s, -1)
	if len(idx) == 0 {
		return s
	}
	out := make([]byte, 0, len(s))
	last := 0
	for _, m := range idx {
		if !singleSep(s[m[0]:m[1]]) || !standalone(s, m[0], m[1]) {
			continue
		}
		out = append(out, s[last:m[0]]...)
		out = append(out, "<phone>"...)
		last = m[1]
	}
	return string(append(out, s[last:]...))
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
	return (start == 0 || !wordish(s[start-1])) && (end == len(s) || !wordish(s[end]))
}

func wordish(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == ':' || c == '/'
}
