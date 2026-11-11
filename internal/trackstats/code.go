package trackstats

import "regexp"

var (
	codeIDRe  = regexp.MustCompile(`^(?:b|cook|fix|hotfix|task)-\d{6}-([0-9a-f]{4})$`)
	codeNumRe = regexp.MustCompile(`/(?:issues|pull)/(\d+)`)
	codeKeyRe = regexp.MustCompile(`/browse/([A-Za-z]+-\d+)/?$`)
	urlRepoRe = regexp.MustCompile(`^https://github\.com/[^/]+/([^/]+)/(?:issues|pull)/\d+`)
)

// Code is the short handle of an item id that the user can type in a prompt: the last 4 hex of a
// track or backlog id, #N for a GitHub issue or PR URL, the key of a Plane browse URL, else "".
func Code(id string) string {
	if m := codeIDRe.FindStringSubmatch(id); m != nil {
		return m[1]
	}
	if m := codeNumRe.FindStringSubmatch(id); m != nil {
		return "#" + m[1]
	}
	if m := codeKeyRe.FindStringSubmatch(id); m != nil {
		return m[1]
	}
	return ""
}

// URLRepo returns <repo> of https://github.com/<owner>/<repo>/(issues|pull)/<n>, "" otherwise.
func URLRepo(u string) string {
	if m := urlRepoRe.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}
