// Package trackstats parses the zenify track logs (log.md and sessions/<id>.log)
// and computes per-item timing and token figures from them. Pure functions, no I/O.
package trackstats

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

const fieldSep = " · "

// Event is one valid log line. F holds the fields after the id (for a cwd line,
// after the type), in file order.
type Event struct {
	At   time.Time
	Type string
	ID   string
	F    []string
}

var (
	idRe    = regexp.MustCompile(`^(cook|fix|hotfix)-\d{6}-[0-9a-f]{4}$`)
	bidRe   = regexp.MustCompile(`^b-\d{6}-[0-9a-f]{4}$`)
	sddRe   = regexp.MustCompile(`^repos/.+/progress\.md:\d+$`)
	hidRe   = regexp.MustCompile(`^[0-9a-f]{8}$`)
	noIDRow = map[string]bool{"backlog": true, "drop": true, "onboard": true, "history": true}
)

func oneOf(v string, set ...string) bool {
	for _, s := range set {
		if v == s {
			return true
		}
	}
	return false
}

func posInt(v string) bool {
	n, err := strconv.Atoi(v)
	return err == nil && n > 0
}

func intOrDash(v string) bool {
	if v == "-" {
		return true
	}
	n, err := strconv.Atoi(v)
	return err == nil && n >= 0
}

// splitLines yields the trimmed non-blank, non-'#' lines of text.
func splitLines(text string) []string {
	var out []string
	for _, raw := range strings.Split(text, "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		out = append(out, raw)
	}
	return out
}

// ParseLog reads log.md. Any non-blank, non-'#' line that is not a valid row is
// counted as malformed.
func ParseLog(text string) (ev []Event, malformed int) {
	for _, raw := range splitLines(text) {
		e, ok := parseLogLine(raw)
		if !ok {
			malformed++
			continue
		}
		ev = append(ev, e)
	}
	return ev, malformed
}

func parseLogLine(raw string) (Event, bool) {
	p := strings.Split(raw, fieldSep)
	if len(p) < 3 {
		return Event{}, false
	}
	at, err := time.Parse(time.RFC3339, p[0])
	if err != nil {
		return Event{}, false
	}
	if noIDRow[p[1]] {
		return parseListRow(at, p)
	}
	if !idRe.MatchString(p[2]) {
		return Event{}, false
	}
	f := p[3:]
	ok := false
	switch p[1] {
	case "start":
		// The optional last field is the backlog id or issue URL the item came from.
		ok = (len(f) == 3 || (len(f) == 4 && validFrom(f[3]))) && oneOf(f[0], "cook", "fix", "hotfix") && f[1] != "" && oneOf(f[2], "S", "M", "L", "-")
	case "est":
		ok = len(f) == 1 && oneOf(f[0], "S", "M", "L")
	case "ship":
		ok = len(f) == 4 && oneOf(f[0], "shipped", "blocked") &&
			oneOf(f[1], "lint", "build", "contract", "behaviour", "review", "-") && f[2] != "" && intOrDash(f[3])
	case "fix":
		ok = len(f) == 3 && posInt(f[0]) && oneOf(f[1], "narrow", "wide") && oneOf(f[2], "test", "no-seam", "-")
	case "ci":
		ok = len(f) == 1 && oneOf(f[0], "green", "red")
	case "end":
		ok = (len(f) == 3 || (len(f) == 4 && f[3] != "")) && f[0] != "" && intOrDash(f[1]) && intOrDash(f[2])
	}
	if !ok {
		return Event{}, false
	}
	return Event{At: at, Type: p[1], ID: p[2], F: f}, true
}

func validFrom(v string) bool {
	return v == "-" || bidRe.MatchString(v) || strings.HasPrefix(v, "http")
}

// parseListRow parses the rows whose third field is not a track id: backlog,
// drop, onboard and history. Event.ID is the bid, hid, dropped id or "memory".
func parseListRow(at time.Time, p []string) (Event, bool) {
	f := p[3:]
	ok := false
	switch p[1] {
	case "backlog":
		// The title is the rest of the line, so it may contain the field separator.
		if len(p) >= 7 {
			f = append(append([]string(nil), p[3:6]...), strings.Join(p[6:], fieldSep))
			ok = bidRe.MatchString(p[2]) && f[0] != "" && oneOf(f[1], "S", "M", "L", "-") && f[2] != "" && f[3] != ""
		}
	case "drop":
		ok = len(f) == 0 && (bidRe.MatchString(p[2]) || sddRe.MatchString(p[2]))
	case "onboard":
		ok = len(f) == 0 && p[2] == "memory"
	case "history":
		ok = len(f) == 4 && hidRe.MatchString(p[2]) && oneOf(f[0], "cook", "fix", "hotfix", "-") && f[1] != "" &&
			intOrDash(f[2]) && f[2] != "-" && validPairs(f[3])
	}
	if !ok {
		return Event{}, false
	}
	return Event{At: at, Type: p[1], ID: p[2], F: f}, true
}

func validPairs(v string) bool {
	if v == "-" {
		return true
	}
	for _, pair := range strings.Split(v, ",") {
		if _, _, _, ok := splitPair(pair); !ok {
			return false
		}
	}
	return true
}

// splitPair splits "<model>:<in>/<out>" at the last ':' (a model id may hold ':').
func splitPair(pair string) (model string, in, out int, ok bool) {
	i := strings.LastIndex(pair, ":")
	if i <= 0 {
		return "", 0, 0, false
	}
	nums := strings.Split(pair[i+1:], "/")
	if len(nums) != 2 {
		return "", 0, 0, false
	}
	in, err1 := strconv.Atoi(nums[0])
	out, err2 := strconv.Atoi(nums[1])
	if err1 != nil || err2 != nil || in < 0 || out < 0 {
		return "", 0, 0, false
	}
	return pair[:i], in, out, true
}

// ParseSession reads sessions/<id>.log. cwd is the path of the first valid cwd
// line; cwd lines are not returned as events.
func ParseSession(_ string, text string) (ev []Event, cwd string, malformed int) {
	for _, raw := range splitLines(text) {
		p := strings.Split(raw, fieldSep)
		if len(p) < 3 {
			malformed++
			continue
		}
		at, err := time.Parse(time.RFC3339, p[0])
		if err != nil {
			malformed++
			continue
		}
		if p[1] == "cwd" {
			if len(p) != 3 || p[2] == "" {
				malformed++
				continue
			}
			if cwd == "" {
				cwd = p[2]
			}
			continue
		}
		f := p[3:]
		ok := false
		switch p[1] {
		case "bind", "unbind":
			ok = len(f) == 0
		case "turn":
			ok = len(f) == 7 && posInt(f[0]) && oneOf(f[1], "main", "sub") && f[2] != "" &&
				intOrDash(f[3]) && intOrDash(f[4]) && intOrDash(f[5]) && intOrDash(f[6])
		case "wait":
			ok = len(f) == 2 && posInt(f[0]) && oneOf(f[1], "ask", "permission", "idle")
		case "phase":
			ok = len(f) == 1 && oneOf(f[0], "design", "diagnose", "scout", "plan", "implement", "fix", "ship")
		}
		if !ok || p[2] == "" {
			malformed++
			continue
		}
		ev = append(ev, Event{At: at, Type: p[1], ID: p[2], F: f})
	}
	return ev, cwd, malformed
}
