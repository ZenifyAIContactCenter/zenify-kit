package obssvc

import "regexp"

var (
	reCred   = regexp.MustCompile(`(://)[^/\s:@]+:[^/\s@]*@`)
	reBearer = regexp.MustCompile(`(?i)bearer\s+\S+`)
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
	return (start == 0 || !wordish(s[start-1])) && (end == len(s) || !wordish(s[end]))
}

func wordish(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == ':' || c == '/'
}
