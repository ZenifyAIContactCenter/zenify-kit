package uiverify

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
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

type rawConfig struct {
	UIVerify *struct {
		RenderPaths []string `json:"renderPaths"`
		Viewports   []int    `json:"viewports"`
	} `json:"uiVerify"`
}

func badConfig(format string, a ...any) error {
	return exitcode.New(exitcode.BadArgs, fmt.Errorf(format, a...))
}

// LoadConfig reads <repo>/.claude/worktree.json. A missing file or missing
// uiVerify key is not an error (declared=false); wt.Load is avoided because it
// errors on a missing file.
func LoadConfig(repo string) (cfg Config, declared bool, err error) {
	path := filepath.Join(repo, ".claude", "worktree.json")
	data, err := os.ReadFile(path)
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
	if raw.UIVerify == nil {
		return Config{}, false, nil
	}
	u := raw.UIVerify
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

func checkRenderPath(p string) error {
	if p == "" {
		return badConfig("uiVerify.renderPaths: empty entry")
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return badConfig("uiVerify.renderPaths: %q must be relative", p)
	}
	for _, seg := range strings.Split(filepath.ToSlash(p), "/") {
		if seg == ".." {
			return badConfig("uiVerify.renderPaths: %q must not contain ..", p)
		}
	}
	return nil
}
