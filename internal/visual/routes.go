package visual

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// Route is one entry of .znf/visual/routes.json.
type Route struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	WaitFor string   `json:"waitFor,omitempty"`
	Mask    []string `json:"mask,omitempty"`
	Aria    string   `json:"aria,omitempty"`
}

// rawRoute keeps waitFor/aria as pointers so "present but empty" is distinguishable from absent.
type rawRoute struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	WaitFor *string  `json:"waitFor"`
	Mask    []string `json:"mask"`
	Aria    *string  `json:"aria"`
}

var routeNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// RoutesPath returns <repo>/.znf/visual/routes.json.
func RoutesPath(repo string) string {
	return filepath.Join(repo, ".znf", "visual", "routes.json")
}

func badArgs(format string, a ...any) error {
	return exitcode.New(exitcode.BadArgs, fmt.Errorf(format, a...))
}

// LoadRoutes reads and validates routes.json; unknown keys are rejected, not ignored.
func LoadRoutes(repo string) ([]Route, error) {
	p := RoutesPath(repo)
	data, err := os.ReadFile(p) //nolint:gosec // G304 -- p is <repo>/.znf/visual/routes.json, repo chosen by the operator
	if errors.Is(err, os.ErrNotExist) {
		return nil, badArgs("%s not found — repo has no visual config", p)
	}
	if err != nil {
		return nil, exitcode.New(exitcode.Fail, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var raw []rawRoute
	if err := dec.Decode(&raw); err != nil {
		return nil, badArgs("%s: %v", p, err)
	}
	rs := make([]Route, len(raw))
	for i, r := range raw {
		rs[i] = Route{Name: r.Name, Path: r.Path, Mask: r.Mask}
		if r.WaitFor != nil {
			if *r.WaitFor == "" {
				return nil, badArgs("routes[%d].waitFor: must not be empty", i)
			}
			rs[i].WaitFor = *r.WaitFor
		}
		if r.Aria != nil {
			if *r.Aria == "" {
				return nil, badArgs("routes[%d].aria: must not be empty", i)
			}
			rs[i].Aria = *r.Aria
		}
	}
	if err := ValidateRoutes(rs); err != nil {
		return nil, err
	}
	return rs, nil
}

// ValidateRoutes checks names/paths are well-formed and unique.
func ValidateRoutes(rs []Route) error {
	names := map[string]bool{}
	paths := map[string]bool{}
	for i, r := range rs {
		if !routeNameRe.MatchString(r.Name) {
			return badArgs("routes[%d].name: %q must match [A-Za-z0-9_-]+", i, r.Name)
		}
		if names[r.Name] {
			return badArgs("routes[%d].name: duplicate %q", i, r.Name)
		}
		names[r.Name] = true
		if len(r.Path) == 0 || r.Path[0] != '/' {
			return badArgs("routes[%d].path: %q must start with /", i, r.Path)
		}
		if paths[r.Path] {
			return badArgs("routes[%d].path: duplicate %q", i, r.Path)
		}
		paths[r.Path] = true
		for _, m := range r.Mask {
			if m == "" {
				return badArgs("routes[%d].mask: selectors must not be empty", i)
			}
		}
	}
	return nil
}

// AddRoute appends r to routes.json. Existing elements are copied as raw JSON so no field of
// theirs (e.g. an empty mask) is lost to a decode/encode round trip.
func AddRoute(repo string, r Route) error {
	p := RoutesPath(repo)
	var existing []Route
	var raw []json.RawMessage
	if _, err := os.Stat(p); err == nil {
		if existing, err = LoadRoutes(repo); err != nil {
			return err
		}
		data, err := os.ReadFile(p) //nolint:gosec // G304 -- p is <repo>/.znf/visual/routes.json, repo chosen by the operator
		if err != nil {
			return exitcode.New(exitcode.Fail, err)
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return badArgs("%s: %v", p, err)
		}
	}
	if err := ValidateRoutes(append(existing, r)); err != nil {
		return err
	}
	nb, err := json.Marshal(r)
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	out, err := json.MarshalIndent(append(raw, nb), "", "  ")
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	if err := os.WriteFile(p, append(out, '\n'), 0o600); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	return nil
}

// BaselineFiles lists the repo-relative baseline files a route produces.
func BaselineFiles(r Route) []string {
	files := []string{".znf/visual/__snapshots__/" + r.Name + "-chromium-linux.png"}
	if r.Aria != "" {
		files = append(files, ".znf/visual/__snapshots__/"+r.Name+".aria.yml")
	}
	return files
}

// GitRunner runs git in dir and returns stdout.
type GitRunner func(dir string, args ...string) (string, error)

// UntrackedBaselines returns on-disk routes.json/baseline files git does not track. Files not on
// disk are skipped: a route without a baseline yet is Playwright's to report. checked=false
// means git could not answer (e.g. not a repo).
func UntrackedBaselines(git GitRunner, repo string, rs []Route) (untracked []string, checked bool) {
	var cands []string
	all := []string{".znf/visual/routes.json"}
	for _, r := range rs {
		all = append(all, BaselineFiles(r)...)
	}
	for _, rel := range all {
		if _, err := os.Stat(filepath.Join(repo, rel)); err == nil {
			cands = append(cands, rel)
		}
	}
	if len(cands) == 0 {
		return nil, true
	}
	out, err := git(repo, append([]string{"-c", "core.quotePath=false", "ls-files", "--"}, cands...)...)
	if err != nil {
		return nil, false
	}
	tracked := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		tracked[strings.TrimSpace(l)] = true
	}
	for _, c := range cands {
		if !tracked[c] {
			untracked = append(untracked, c)
		}
	}
	return untracked, true
}
