// Package uiverify is the deterministic fail-closed engine behind
// `zenify ui-verify record|check`. It decides whether a diff touches
// rendering files, records a UI-verify artifact for the current working-tree
// fingerprint, and validates that a recorded artifact still matches HEAD.
package uiverify

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// renderRe matches a path that renders UI: an extension known to be a
// component/style file, or a directory segment that conventionally holds
// screens/components. FR-2.1 — keep this exact pattern in sync with the brief.
var renderRe = regexp.MustCompile(`\.(tsx|jsx|vue|svelte|css|scss|less)$|(^|/)(components?|pages?|views?)/`)

// testFileRe matches a test file: it renders nothing a person sees, so it never
// requires a UI verdict even when it sits under a UI directory or ends in .tsx.
var testFileRe = regexp.MustCompile(`(^|/)__tests__/|\.(test|spec)\.[cm]?[jt]sx?$`)

// unsafeFileRe matches what may not appear in a screenshot file name: a
// route-shaped screen ("chat/room") would otherwise become a missing subdir.
var unsafeFileRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// screenFileName is the screenshot file name for a screen. A name that had to
// be rewritten carries a short hash of the raw name, so "chat/room" and
// "chat-room" never overwrite each other's screenshot.
func screenFileName(fp, screen string, viewport int) string {
	safe := unsafeFileRe.ReplaceAllString(screen, "-")
	if safe != screen {
		safe = fmt.Sprintf("%s-%x", safe, sha256.Sum256([]byte(screen)))[:len(safe)+9]
	}
	return fmt.Sprintf("%s-%s-%d.png", fp, safe, viewport)
}

// waiverMarker: a line containing this in a diff or the working tree waives
// the gate; the reason is whatever follows the ':'. FR-2.3.
const waiverMarker = "znf:ui-verify-ok:"

// Screen is one verified screen/route at one viewport within an Artifact.
type Screen struct {
	Screen     string     `json:"screen"`
	Viewport   int        `json:"viewport"`
	Screenshot string     `json:"screenshot"` // basename under .znf/ui-verify/
	Layout     Layout     `json:"layout"`
	Overflows  []Overflow `json:"overflows,omitempty"`
	FailReason string     `json:"failReason,omitempty"`
	Verdict    string     `json:"verdict"` // "pass" | "fail", computed by Record
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
	// RunGit's real (non-test) implementation MUST return raw stdout —
	// including trailing newlines — for every call Fingerprint feeds into
	// the hash (rev-parse/diff/status): do NOT strings.TrimSpace(out) on
	// those. Only the final hash-object result is trimmed (inside
	// Fingerprint itself). Trimming any of the three inputs would diverge
	// the fingerprint from ship's fp() and silently break check↔record
	// matching (a `record`-ed artifact would never validate against the
	// fp a later `check` computes).
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
// The three RunGit outputs are concatenated RAW, with no TrimSpace — see the
// warning on Deps.RunGit. Only the hash-object result is trimmed, below.
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

// RenderSet is the union of files changed on this branch (since its merge-base
// with base, so later commits on base are not counted) and files dirty in the
// working tree (including untracked), filtered to those that render UI.
// core.quotePath=false keeps non-ASCII names unquoted so the patterns match.
// FR-2.1.
func RenderSet(d Deps, repo, base string) ([]string, error) {
	cfg, declared, err := LoadConfig(repo)
	if err != nil {
		return nil, err
	}
	diffOut, err := d.RunGit(repo, "-c", "core.quotePath=false", "diff", "--name-only", base+"...HEAD")
	if err != nil {
		return nil, exitcode.New(exitcode.BadArgs, fmt.Errorf("git diff --name-only %s...HEAD: %w", base, err))
	}
	statusOut, err := d.RunGit(repo, "-c", "core.quotePath=false", "status", "--porcelain", "-uall")
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

	renders := renderRe.MatchString
	if declared && len(cfg.RenderPaths) > 0 {
		renders = func(p string) bool { return underAny(p, cfg.RenderPaths) }
	}

	var out []string
	for p := range set {
		if renders(p) && !testFileRe.MatchString(p) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// underAny reports whether p is one of prefixes or sits under one, matching
// whole path segments ("lib" must not match "libx/a.js").
func underAny(p string, prefixes []string) bool {
	for _, pre := range prefixes {
		pre = strings.TrimSuffix(pre, "/")
		if p == pre || strings.HasPrefix(p, pre+"/") {
			return true
		}
	}
	return false
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

// RecordInput is what a caller supplies to Record; the verdict is computed
// from MeasurementJSON and FailReason, never passed in.
type RecordInput struct {
	Screen          string
	Viewport        int
	ScreenshotSrc   string
	MeasurementJSON string
	FailReason      string
}

func badArg(format string, a ...any) error {
	return exitcode.New(exitcode.BadArgs, fmt.Errorf(format, a...))
}

// Record validates the screenshot and measurement, computes the verdict (fail
// on any overflow or a non-empty FailReason), copies the screenshot into
// .znf/ui-verify/ and upserts the screen, keyed by (Screen, Viewport), into
// <fp>.json. Validation errors are BadArgs and touch no disk; I/O errors after
// validation are Fail.
func Record(d Deps, repo string, in RecordInput) (Screen, error) {
	info, err := os.Stat(in.ScreenshotSrc)
	if err != nil {
		return Screen{}, badArg("screenshot not found: %s: %w", in.ScreenshotSrc, err)
	}
	if info.Size() == 0 {
		return Screen{}, badArg("screenshot is 0 bytes: %s", in.ScreenshotSrc)
	}
	layout, err := ParseLayout(in.MeasurementJSON)
	if err != nil {
		return Screen{}, err
	}
	if in.Viewport < minViewport || in.Viewport > maxViewport {
		return Screen{}, badArg("viewport %d out of range [%d,%d]", in.Viewport, minViewport, maxViewport)
	}
	if layout.InnerWidth != float64(in.Viewport) {
		return Screen{}, badArg("measurement innerWidth %v does not match viewport %d", layout.InnerWidth, in.Viewport)
	}

	s := Screen{
		Screen:     in.Screen,
		Viewport:   in.Viewport,
		Layout:     layout,
		Overflows:  Overflows(layout),
		FailReason: strings.TrimSpace(in.FailReason),
		Verdict:    "pass",
	}
	if len(s.Overflows) > 0 || s.FailReason != "" {
		s.Verdict = "fail"
	}

	fp, err := Fingerprint(d, repo)
	if err != nil {
		return Screen{}, err
	}

	dir := artifactDir(repo)
	if err := ensureArtifactDir(dir); err != nil {
		return Screen{}, exitcode.New(exitcode.Fail, err)
	}

	destName := screenFileName(fp, s.Screen, s.Viewport)
	if err := copyFile(in.ScreenshotSrc, filepath.Join(dir, destName)); err != nil {
		return Screen{}, exitcode.New(exitcode.Fail, fmt.Errorf("copy screenshot: %w", err))
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
		if sc.Screen == s.Screen && sc.Viewport == s.Viewport {
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
		return Screen{}, exitcode.New(exitcode.Fail, err)
	}
	if err := os.WriteFile(artifactPath, b, 0o600); err != nil {
		return Screen{}, exitcode.New(exitcode.Fail, err)
	}
	return s, nil
}

// Validate lists why <fp>.json cannot satisfy the gate; no problems means it
// does. Every entry must have a viewport, a pass verdict and a non-empty
// screenshot file, and when viewports is non-empty every screen must have an
// entry at each of them. A legacy or unparsable artifact is a problem, never a
// panic. FR-3.1, FR-3.3, FR-3.4.
func Validate(repo, fp string, viewports []int) (screens int, problems []string) {
	dir := artifactDir(repo)
	b, err := os.ReadFile(filepath.Join(dir, fp+".json")) //nolint:gosec // G304 -- path built from fp, not user input
	if err != nil {
		return 0, []string{"no artifact for fp=" + fp}
	}
	var art Artifact
	if err := json.Unmarshal(b, &art); err != nil {
		return 0, []string{"unreadable artifact for fp=" + fp}
	}
	if len(art.Screens) == 0 {
		return 0, []string{"no screens in artifact for fp=" + fp}
	}

	var names []string
	have := map[string]map[int]bool{}
	for _, s := range art.Screens {
		if s.Viewport == 0 || (s.Verdict != "pass" && s.Verdict != "fail") {
			problems = append(problems, s.Screen+": invalid entry")
			continue
		}
		if have[s.Screen] == nil {
			have[s.Screen] = map[int]bool{}
			names = append(names, s.Screen)
		}
		have[s.Screen][s.Viewport] = true
		label := fmt.Sprintf("%s@%d", s.Screen, s.Viewport)
		if s.Verdict == "fail" {
			problems = append(problems, fmt.Sprintf("%s: fail (%s)", label, failDetail(s)))
			continue
		}
		info, err := os.Stat(filepath.Join(dir, s.Screenshot))
		if err != nil || info.Size() == 0 {
			problems = append(problems, label+": screenshot missing")
		}
	}
	for _, name := range names {
		for _, vp := range viewports {
			if !have[name][vp] {
				problems = append(problems, fmt.Sprintf("%s: missing viewport %d", name, vp))
			}
		}
	}
	return len(art.Screens), problems
}

// failDetail renders a failing entry's overflows ("right 5px, bottom 3px"),
// falling back to the recorded reason when there is no overflow.
func failDetail(s Screen) string {
	if len(s.Overflows) == 0 {
		return s.FailReason
	}
	parts := make([]string, len(s.Overflows))
	for i, o := range s.Overflows {
		parts[i] = o.Edge + " " + strconv.FormatFloat(o.Px, 'g', -1, 64) + "px"
	}
	return strings.Join(parts, ", ")
}

// Check orchestrates the gate: no rendering files changed → not_required;
// a waiver marker present → waived; an artifact for the current fingerprint
// with every declared viewport recorded and passing → verified; otherwise →
// required (fail-closed). FR-2.2..2.5, FR-3.1.
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

	cfg, _, err := LoadConfig(repo)
	if err != nil {
		return "", "", err
	}
	fp, err := Fingerprint(d, repo)
	if err != nil {
		return "", "", err
	}
	n, problems := Validate(repo, fp, cfg.Viewports)
	if len(problems) == 0 {
		msg = fmt.Sprintf("verified (%d screen(s), fp=%s)", n, fp)
		return "verified", msg, nil
	}

	msg = fmt.Sprintf("ui-verify required — %s (rendering files: %s)",
		strings.Join(problems, "; "), strings.Join(rs, ", "))
	return "required", msg, exitcode.New(exitcode.Fail, fmt.Errorf("%s", msg))
}
