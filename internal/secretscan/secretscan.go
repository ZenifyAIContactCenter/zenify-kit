// Package secretscan wraps gitleaks as a library to detect secrets, reporting
// only location + rule, never the raw value (FR-041).
package secretscan

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zricethezav/gitleaks/v8/detect"
)

type Finding struct {
	File      string
	RuleID    string
	StartLine int
	Redacted  string
}

type Scanner interface {
	ScanText(name, content string) []Finding
	ScanPath(root string) ([]Finding, error)
}

type gitleaksScanner struct {
	d *detect.Detector
}

func New() (Scanner, error) {
	d, err := detect.NewDetectorDefaultConfig()
	if err != nil {
		return nil, fmt.Errorf("secretscan: init detector: %w", err)
	}
	return &gitleaksScanner{d: d}, nil
}

// redact always returns a constant mask, containing none of the secret's
// characters (FR-041: report only file+rule-id+line, never leak the secret value even partially).
func redact(_ string) string {
	return "(redacted)"
}

func (g *gitleaksScanner) ScanText(name, content string) []Finding {
	raw := g.d.DetectString(content)
	out := make([]Finding, 0, len(raw))
	for _, r := range raw {
		out = append(out, Finding{
			File:      name,
			RuleID:    r.RuleID,
			StartLine: r.StartLine,
			Redacted:  redact(r.Secret),
		})
	}
	return out
}

func (g *gitleaksScanner) ScanPath(root string) ([]Finding, error) {
	var out []Finding
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip the one-off error, don't stop the whole tree walk
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		b, rerr := os.ReadFile(path) //nolint:gosec // G304 -- path comes from filepath.WalkDir over the caller-specified scan root, not attacker-controlled input
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, g.ScanText(rel, string(b))...)
		return nil
	})
	return out, err
}

// Staged scans the staged diff + staged file names. deny=true if there's a secret or
// settings.local.json is staged. err only when the scan step itself fails (caller fails open).
func Staged(repoDir string, s Scanner) (bool, string, error) {
	// 1. Staged file names: block settings.local.json.
	names, err := stagedNames(repoDir)
	if err != nil {
		return false, "", err
	}
	for _, n := range names {
		if filepath.Base(n) == "settings.local.json" {
			return true, "🚫 [git-guard] BLOCKED — 'settings.local.json' is staged (contains secrets, must not be committed).", nil
		}
	}
	// 2. Staged diff content.
	diff, err := stagedDiff(repoDir)
	if err != nil {
		return false, "", err
	}
	fs := s.ScanText("<staged>", diff)
	if len(fs) > 0 {
		return true, fmt.Sprintf("🚫 [git-guard] BLOCKED — staged diff contains a suspected secret (%s, line %d). Remove it and re-stage.", fs[0].RuleID, fs[0].StartLine), nil
	}
	return false, "", nil
}

func stagedNames(repoDir string) ([]string, error) {
	out, err := gitOut(repoDir, "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l != "" {
			names = append(names, l)
		}
	}
	return names, nil
}

func stagedDiff(repoDir string) (string, error) {
	return gitOut(repoDir, "diff", "--cached")
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...) //nolint:gosec // G204 -- fixed "git" binary, args are internally-computed git subcommands, not attacker-controlled shell input
	cmd.Dir = dir
	b, err := cmd.Output()
	return string(b), err
}
