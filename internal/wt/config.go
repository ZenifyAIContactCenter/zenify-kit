// Package wt is the Go reimplementation of the bash `wt` worktree tool. C1 is
// read-only: it parses config, allocates a deterministic port, and reads state.
// All mutation (creating worktrees, writing state) lands in a later slice.
package wt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is the resolved .claude/worktree.json for one repo. Keys absent on
// disk take the documented defaults (matching the bash wt).
type Config struct {
	Abbrev        string
	BaseRef       string
	WorktreeDir   string
	PortEnv       string
	Deps          string
	Install       string
	User          string
	PortRange     [2]int
	Copy          []string
	EnvFile       string
	HotfixBaseRef string
	RedisPrefixEnv string
}

// rawConfig mirrors the on-disk JSON. PortRange is deferred to json.RawMessage
// because it appears BOTH as an array [lo,hi] (real files) and as a legacy
// "lo hi" string (old default).
type rawConfig struct {
	Abbrev        string          `json:"abbrev"`
	BaseRef       string          `json:"baseRef"`
	WorktreeDir   string          `json:"worktreeDir"`
	PortEnv       string          `json:"portEnv"`
	Deps          string          `json:"deps"`
	Install       string          `json:"install"`
	User          string          `json:"user"`
	PortRange     json.RawMessage `json:"portRange"`
	Copy          []string        `json:"copy"`
	EnvFile       string          `json:"envFile"`
	HotfixBaseRef string          `json:"hotfixBaseRef"`
	RedisPrefixEnv string         `json:"redisPrefixEnv"`
}

// Load reads <repoRoot>/.claude/worktree.json and resolves defaults. A missing
// file is an error: wt cannot operate on a repo that does not declare one.
func Load(repoRoot string) (*Config, error) {
	path := filepath.Join(repoRoot, ".claude", "worktree.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("wt: read %s: %w", path, err)
	}
	var raw rawConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("wt: %s is not valid JSON: %w", path, err)
	}
	pr, err := parsePortRange(raw.PortRange)
	if err != nil {
		return nil, fmt.Errorf("wt: %s portRange: %w", path, err)
	}
	c := &Config{
		Abbrev:        orDefault(raw.Abbrev, filepath.Base(repoRoot)),
		BaseRef:       orDefault(raw.BaseRef, "origin/main"),
		WorktreeDir:   orDefault(raw.WorktreeDir, ".worktrees/"),
		PortEnv:       orDefault(raw.PortEnv, "PORT"),
		Deps:          orDefault(raw.Deps, "install"),
		Install:       raw.Install,
		User:          orDefault(raw.User, "namph"),
		PortRange:     pr,
		Copy:          raw.Copy,
		EnvFile:       raw.EnvFile,
		HotfixBaseRef: raw.HotfixBaseRef,
		RedisPrefixEnv: raw.RedisPrefixEnv,
	}
	return c, nil
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// parsePortRange accepts [lo,hi], "lo hi", or empty (→ default [3100,3999]).
func parsePortRange(raw json.RawMessage) ([2]int, error) {
	// Absent key, or an explicit JSON null, both mean "use the default". Without
	// the null check, `"portRange": null` unmarshals into a nil []int and falls
	// into the array branch as "0 elements", masking the default with an error.
	if len(raw) == 0 || string(raw) == "null" {
		return [2]int{3100, 3999}, nil
	}
	var arr []int
	if err := json.Unmarshal(raw, &arr); err == nil {
		if len(arr) != 2 {
			return [2]int{}, fmt.Errorf("array needs exactly 2 elements, got %d", len(arr))
		}
		return [2]int{arr[0], arr[1]}, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		f := strings.Fields(s)
		if len(f) != 2 {
			return [2]int{}, fmt.Errorf("string form needs \"lo hi\", got %q", s)
		}
		lo, err1 := strconv.Atoi(f[0])
		hi, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			return [2]int{}, fmt.Errorf("non-numeric bounds in %q", s)
		}
		return [2]int{lo, hi}, nil
	}
	return [2]int{}, fmt.Errorf("must be [lo,hi] or \"lo hi\"")
}
