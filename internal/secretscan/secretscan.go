// Package secretscan bọc gitleaks làm library để phát hiện secret, chỉ báo
// vị trí + rule, không bao giờ trả value nguyên (FR-041).
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

// redact luôn trả một mask hằng số, không chứa bất kỳ ký tự nào của secret
// (FR-041: chỉ báo file+rule-id+line, không lộ giá trị secret dù một phần).
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
			return nil // bỏ qua lỗi lẻ, không dừng cả cây
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

// Staged quét diff đã stage + tên file staged. deny=true nếu có secret hoặc
// settings.local.json bị stage. err chỉ khi bước quét lỗi (caller fail-open).
func Staged(repoDir string, s Scanner) (bool, string, error) {
	// 1. Tên file staged: chặn settings.local.json.
	names, err := stagedNames(repoDir)
	if err != nil {
		return false, "", err
	}
	for _, n := range names {
		if filepath.Base(n) == "settings.local.json" {
			return true, "🚫 [git-guard] BLOCKED — 'settings.local.json' bị stage (chứa secret, không được commit).", nil
		}
	}
	// 2. Nội dung staged diff.
	diff, err := stagedDiff(repoDir)
	if err != nil {
		return false, "", err
	}
	fs := s.ScanText("<staged>", diff)
	if len(fs) > 0 {
		return true, fmt.Sprintf("🚫 [git-guard] BLOCKED — staged diff chứa secret khả nghi (%s, dòng %d). Gỡ rồi re-stage.", fs[0].RuleID, fs[0].StartLine), nil
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
