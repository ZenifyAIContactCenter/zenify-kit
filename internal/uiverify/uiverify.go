// Package uiverify is the deterministic fail-closed engine behind
// `zenify ui-verify record|check`. It decides whether a diff touches
// rendering files, records a UI-verify artifact for the current working-tree
// fingerprint, and validates that a recorded artifact still matches HEAD.
package uiverify

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// renderRe matches a path that renders UI: an extension known to be a
// component/style file, or a directory segment that conventionally holds
// screens/components. FR-2.1 — keep this exact pattern in sync with the brief.
var renderRe = regexp.MustCompile(`\.(tsx|jsx|vue|svelte|css|scss|less)$|components?/|pages?/|views?/`)

// waiverMarker: a line containing this in a diff or the working tree waives
// the gate; the reason is whatever follows the ':'. FR-2.3.
const waiverMarker = "znf:ui-verify-ok:"

// Measurement is the objective layout measurement captured for one screen
// (changed element vs its own container box).
type Measurement struct {
	ChildRight     float64 `json:"child_right"`
	ContainerRight float64 `json:"container_right"`
	PaddingRight   float64 `json:"padding_right"`
}

// Screen is one verified screen/route within an Artifact.
type Screen struct {
	Screen      string      `json:"screen"`
	Screenshot  string      `json:"screenshot"` // basename under .znf/ui-verify/
	Measurement Measurement `json:"measurement"`
	Verdict     string      `json:"verdict"` // "pass" | "fail"
}

// Artifact is the recorded UI-verify result for one working-tree fingerprint.
type Artifact struct {
	FP         string   `json:"fp"`
	RecordedAt string   `json:"recorded_at"`
	Screens    []Screen `json:"screens"`
}

// Deps injects everything uiverify needs from the outside world so it stays
// testable without a real git binary or clock.
type Deps struct {
	RunGit func(dir string, args ...string) (string, error)
	Now    func() time.Time
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// artifactDir is where every ui-verify artifact + screenshot lives, gitignored
// so it never dirties the tree (mirrors the SDD workspace pattern).
func artifactDir(repo string) string {
	return filepath.Join(repo, ".znf", "ui-verify")
}

// Fingerprint reproduces ship's fp() pipeline exactly: hash of
// {HEAD; diff HEAD; status --porcelain -uall} concatenated in that order,
// truncated to 10 hex chars. FR-2.6.
//
// git hash-object --stdin needs stdin content that Deps.RunGit's signature
// (dir + args, no stdin) cannot carry as a real git flag would. The
// convention here is that the content to hash is passed as the trailing
// argument after "--stdin"; the production RunGit (wired in Task 2) special-
// cases this pair and pipes that argument to the real git process's stdin
// instead of passing it as a CLI arg. This keeps Fingerprint fully testable
// with an injected RunGit while still describing exactly what the real
// implementation must do.
func Fingerprint(d Deps, repo string) (string, error) {
	head, err := d.RunGit(repo, "rev-parse", "HEAD")
	if err != nil {
		return "", exitcode.New(exitcode.BadArgs, fmt.Errorf("git rev-parse HEAD: %w", err))
	}
	diff, err := d.RunGit(repo, "diff", "HEAD")
	if err != nil {
		return "", exitcode.New(exitcode.BadArgs, fmt.Errorf("git diff HEAD: %w", err))
	}
	status, err := d.RunGit(repo, "status", "--porcelain", "-uall")
	if err != nil {
		return "", exitcode.New(exitcode.BadArgs, fmt.Errorf("git status --porcelain -uall: %w", err))
	}
	content := head + diff + status
	hash, err := d.RunGit(repo, "hash-object", "--stdin", content)
	if err != nil {
		return "", exitcode.New(exitcode.BadArgs, fmt.Errorf("git hash-object --stdin: %w", err))
	}
	hash = strings.TrimSpace(hash)
	if len(hash) > 10 {
		hash = hash[:10]
	}
	return hash, nil
}

// parsePorcelainPath extracts the path from one `git status --porcelain`
// line: strip the 2-char XY status + separating space, then take the target
// side of a rename ("old -> new").
func parsePorcelainPath(line string) string {
	if len(line) <= 3 {
		return ""
	}
	path := strings.TrimSpace(line[3:])
	if idx := strings.Index(path, " -> "); idx >= 0 {
		path = path[idx+len(" -> "):]
	}
	return strings.Trim(path, `"`)
}

// RenderSet is the union of files changed since base and files dirty in the
// working tree (including untracked), filtered to those that render UI.
// FR-2.1.
func RenderSet(d Deps, repo, base string) ([]string, error) {
	diffOut, err := d.RunGit(repo, "diff", "--name-only", base+"..HEAD")
	if err != nil {
		return nil, exitcode.New(exitcode.BadArgs, fmt.Errorf("git diff --name-only %s..HEAD: %w", base, err))
	}
	statusOut, err := d.RunGit(repo, "status", "--porcelain", "-uall")
	if err != nil {
		return nil, exitcode.New(exitcode.BadArgs, fmt.Errorf("git status --porcelain -uall: %w", err))
	}

	set := map[string]struct{}{}
	for _, line := range strings.Split(diffOut, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			set[line] = struct{}{}
		}
	}
	for _, line := range strings.Split(statusOut, "\n") {
		line = strings.TrimRight(line, "\r")
		if path := parsePorcelainPath(line); path != "" {
			set[path] = struct{}{}
		}
	}

	var out []string
	for p := range set {
		if renderRe.MatchString(p) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// findWaiver scans text line by line for waiverMarker and returns the
// trimmed reason after the ':'.
func findWaiver(text string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		if idx := strings.Index(line, waiverMarker); idx >= 0 {
			reason := strings.TrimSpace(line[idx+len(waiverMarker):])
			return reason, true
		}
	}
	return "", false
}

// Waiver checks both the committed diff since base and the current working
// tree for waiverMarker. FR-2.3.
func Waiver(d Deps, repo, base string) (string, bool) {
	committed, _ := d.RunGit(repo, "diff", base+"..HEAD")
	if reason, ok := findWaiver(committed); ok {
		return reason, true
	}
	working, _ := d.RunGit(repo, "diff", "HEAD")
	return findWaiver(working)
}

// copyFile copies src to dst, creating/truncating dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // G304 -- src is a caller-supplied screenshot path
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst) //nolint:gosec // G304 -- dst is computed from fp+screen, not user input
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// ensureArtifactDir creates dir/.znf/ui-verify + a self-ignoring .gitignore
// ("*") if not already present, so recorded artifacts never dirty the tree.
func ensureArtifactDir(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	gi := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gi); os.IsNotExist(err) {
		if err := os.WriteFile(gi, []byte("*\n"), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// Record validates the screenshot, computes the current fingerprint, copies
// the screenshot into .znf/ui-verify/, and upserts s into <fp>.json (an
// existing entry for the same Screen name is replaced). FR-1.1..1.3.
func Record(d Deps, repo string, s Screen, screenshotSrc string) error {
	info, err := os.Stat(screenshotSrc)
	if err != nil {
		return exitcode.New(exitcode.BadArgs, fmt.Errorf("screenshot not found: %s: %w", screenshotSrc, err))
	}
	if info.Size() == 0 {
		return exitcode.New(exitcode.BadArgs, fmt.Errorf("screenshot is 0 bytes: %s", screenshotSrc))
	}

	fp, err := Fingerprint(d, repo)
	if err != nil {
		return err
	}

	dir := artifactDir(repo)
	if err := ensureArtifactDir(dir); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}

	destName := fmt.Sprintf("%s-%s.png", fp, s.Screen)
	dest := filepath.Join(dir, destName)
	if err := copyFile(screenshotSrc, dest); err != nil {
		return exitcode.New(exitcode.Fail, fmt.Errorf("copy screenshot: %w", err))
	}
	s.Screenshot = destName

	artifactPath := filepath.Join(dir, fp+".json")
	var art Artifact
	if b, err := os.ReadFile(artifactPath); err == nil { //nolint:gosec // G304 -- path built from fp, not user input
		_ = json.Unmarshal(b, &art)
	}
	art.FP = fp
	art.RecordedAt = d.now().UTC().Format(time.RFC3339)

	upserted := false
	for i, sc := range art.Screens {
		if sc.Screen == s.Screen {
			art.Screens[i] = s
			upserted = true
			break
		}
	}
	if !upserted {
		art.Screens = append(art.Screens, s)
	}

	b, err := json.MarshalIndent(art, "", "  ")
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	if err := os.WriteFile(artifactPath, b, 0o600); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	return nil
}

// Validate reports whether <fp>.json exists, has at least one screen, and
// every screen's screenshot file exists and is non-zero bytes. The three
// measurement floats are always present because the CLI (Task 2) makes those
// flags required, so Validate does not need to distinguish a 0.0 measurement
// from "absent". FR-2.4.
func Validate(repo, fp string) (int, bool) {
	dir := artifactDir(repo)
	artifactPath := filepath.Join(dir, fp+".json")
	b, err := os.ReadFile(artifactPath) //nolint:gosec // G304 -- path built from fp, not user input
	if err != nil {
		return 0, false
	}
	var art Artifact
	if err := json.Unmarshal(b, &art); err != nil {
		return 0, false
	}
	if len(art.Screens) == 0 {
		return 0, false
	}
	for _, s := range art.Screens {
		info, err := os.Stat(filepath.Join(dir, s.Screenshot))
		if err != nil || info.Size() == 0 {
			return len(art.Screens), false
		}
	}
	return len(art.Screens), true
}

// Check orchestrates the gate: no rendering files changed → not_required;
// a waiver marker present → waived; a valid artifact for the current
// fingerprint → verified; otherwise → required (fail-closed).
// FR-2.2..2.5, FR-3.1 (a stale fp means Validate reads a <fp>.json that
// doesn't exist, which falls straight through to required).
func Check(d Deps, repo, base string) (state string, msg string, err error) {
	rs, err := RenderSet(d, repo, base)
	if err != nil {
		return "", "", err
	}
	if len(rs) == 0 {
		return "not_required", "no rendering files changed", nil
	}

	if reason, ok := Waiver(d, repo, base); ok {
		state = fmt.Sprintf("waived: %s", reason)
		return state, state, nil
	}

	fp, err := Fingerprint(d, repo)
	if err != nil {
		return "", "", err
	}
	n, ok := Validate(repo, fp)
	if ok {
		msg = fmt.Sprintf("verified (%d screen(s), fp=%s)", n, fp)
		return "verified", msg, nil
	}

	msg = fmt.Sprintf("ui-verify required — no valid artifact for fp=%s (rendering files: %s)",
		fp, strings.Join(rs, ", "))
	return "required", msg, exitcode.New(exitcode.Fail, fmt.Errorf("%s", msg))
}
