package trackstats

import (
	"errors"
	"regexp"
	"strings"
)

// Candidate is one item a short code may name: its full id, short code, repo and title.
type Candidate struct {
	ID, Code, Repo, Title string
}

var (
	ErrNoMatch   = errors.New("no item matches")
	ErrAmbiguous = errors.New("ref matches several items")

	refHexRe     = regexp.MustCompile(`^[0-9a-f]{4}$`)
	refNumRe     = regexp.MustCompile(`^#\d+$`)
	refRepoNumRe = regexp.MustCompile(`^([^#\s]+)(#\d+)$`)
	refKeyRe     = regexp.MustCompile(`^[A-Za-z]+-\d+$`)
)

// IsFullID reports whether ref is already a full item id, which needs no lookup.
func IsFullID(ref string) bool { return ValidPlanID(ref) }

// Resolve finds the candidate a short code names: xxxx (bid or track id suffix), #n, <repo>#n or
// a Plane KEY-n. One match returns it; none returns ErrNoMatch; several return them with
// ErrAmbiguous. Candidates sharing an id count once.
func Resolve(ref string, cands []Candidate) (Candidate, []Candidate, error) {
	var match func(Candidate) bool
	switch {
	case refHexRe.MatchString(ref), refNumRe.MatchString(ref):
		match = func(c Candidate) bool { return c.Code == ref }
	case refRepoNumRe.MatchString(ref):
		m := refRepoNumRe.FindStringSubmatch(ref)
		match = func(c Candidate) bool { return c.Code == m[2] && c.Repo == m[1] }
	case refKeyRe.MatchString(ref):
		match = func(c Candidate) bool { return strings.EqualFold(c.Code, ref) }
	default:
		return Candidate{}, nil, ErrNoMatch
	}
	var matches []Candidate
	seen := map[string]bool{}
	for _, c := range cands {
		if c.Code == "" || seen[c.ID] || !match(c) {
			continue
		}
		seen[c.ID] = true
		matches = append(matches, c)
	}
	switch len(matches) {
	case 0:
		return Candidate{}, nil, ErrNoMatch
	case 1:
		return matches[0], nil, nil
	}
	return Candidate{}, matches, ErrAmbiguous
}
