package uiverify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

const (
	minViewport = 320
	maxViewport = 3840
)

// Config is the per-repo uiVerify block of .claude/worktree.json.
type Config struct {
	RenderPaths []string
	Viewports   []int
}

// rawConfig keeps the rest of worktree.json lenient (it has many other keys);
// only the uiVerify block is decoded strictly.
type rawConfig struct {
	UIVerify json.RawMessage `json:"uiVerify"`
}

type rawUIVerify struct {
	RenderPaths []string `json:"renderPaths"`
	Viewports   []int    `json:"viewports"`
}

func badConfig(format string, a ...any) error {
	return exitcode.New(exitcode.BadArgs, fmt.Errorf(format, a...))
}

// LoadConfig reads <repo>/.claude/worktree.json. A missing file or missing
// uiVerify key is not an error (declared=false); wt.Load is avoided because it
// errors on a missing file.
func LoadConfig(repo string) (cfg Config, declared bool, err error) {
	path := filepath.Join(repo, ".claude", "worktree.json")
	data, err := os.ReadFile(path) //nolint:gosec // G304 -- fixed .claude/worktree.json under the operator's own --repo
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, badConfig("read worktree.json: %w", err)
	}
	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, false, badConfig("worktree.json: invalid JSON: %w", err)
	}
	if len(raw.UIVerify) == 0 || bytes.Equal(raw.UIVerify, []byte("null")) {
		return Config{}, false, nil
	}
	// A misspelled key ("viewport") would otherwise silently drop the policy.
	var u rawUIVerify
	dec := json.NewDecoder(bytes.NewReader(raw.UIVerify))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&u); err != nil {
		return Config{}, false, badConfig("worktree.json: uiVerify: %w", err)
	}
	if u.RenderPaths != nil {
		if len(u.RenderPaths) == 0 {
			return Config{}, false, badConfig("uiVerify.renderPaths must not be empty when present")
		}
		for _, p := range u.RenderPaths {
			if err := checkRenderPath(p); err != nil {
				return Config{}, false, err
			}
		}
	}
	seen := map[int]bool{}
	for _, v := range u.Viewports {
		if v < minViewport || v > maxViewport {
			return Config{}, false, badConfig("uiVerify.viewports: %d outside [%d, %d]", v, minViewport, maxViewport)
		}
		if seen[v] {
			return Config{}, false, badConfig("uiVerify.viewports: duplicate %d", v)
		}
		seen[v] = true
	}
	return Config{RenderPaths: u.RenderPaths, Viewports: u.Viewports}, true, nil
}

// checkRenderPath accepts only a literal, clean, relative prefix: underAny
// compares raw strings, so "./src", "src//" or a glob would match nothing and
// silently turn the gate off.
func checkRenderPath(p string) error {
	trimmed := strings.TrimSuffix(p, "/")
	switch {
	case trimmed == "" || trimmed == ".":
		return badConfig("uiVerify.renderPaths: %q is not a directory or file prefix", p)
	case strings.HasPrefix(p, "/") || filepath.IsAbs(p):
		return badConfig("uiVerify.renderPaths: %q must be relative", p)
	case strings.ContainsAny(p, `*?[\`):
		return badConfig("uiVerify.renderPaths: %q must be a literal path prefix, not a glob", p)
	case strings.HasPrefix(p, "./") || path.Clean(trimmed) != trimmed:
		return badConfig("uiVerify.renderPaths: %q must be a clean path (no ./, //, or .. segments)", p)
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == ".." {
			return badConfig("uiVerify.renderPaths: %q must not contain ..", p)
		}
	}
	return nil
}
