// Package wt is the Go reimplementation of the bash `wt` worktree tool. C1 is
// read-only: it parses config, allocates a deterministic port, and reads state.
// All mutation (creating worktrees, writing state) lands in a later slice.
package wt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Config is the resolved .claude/worktree.json for one repo. Keys absent on
// disk take the documented defaults (matching the bash wt).
type Config struct {
	Abbrev         string
	BaseRef        string
	WorktreeDir    string
	PortEnv        string
	Deps           string
	DepsDir        string
	Install        string
	User           string
	PortRange      [2]int
	PortCount      int
	Copy           []string
	EnvFile        string
	HotfixBaseRef  string
	RedisPrefixEnv string
	Peers          map[string]PeerSpec

	HotfixBaseStrategy string
	GateSharedStore    bool
	GateAccessPatterns []string
	GateDBAccessor     string

	// Variants are named checkout shapes a repo offers to `wt new --variant`
	// (e.g. a light JS-only checkout vs a full native one). DefaultVariant is
	// applied when no --variant is passed; empty = no variant, plain checkout.
	Variants       map[string]Variant
	DefaultVariant string
}

// Variant is one entry of the variants map: how `wt new` shapes the checkout.
type Variant struct {
	// SparseExclude lists repo-root-relative paths (files or directories) hidden from the worktree via
	// a per-worktree non-cone sparse-checkout (the main checkout is untouched).
	SparseExclude []string `json:"sparseExclude"`
	// Submodules are initialised (`git submodule update --init`) in the worktree.
	Submodules []string `json:"submodules"`
	// Setup is a shell command run in the worktree after deps are in place.
	Setup string `json:"setup"`
}

// PeerSpec is one entry of the peers map, keyed by the env var it sets.
type PeerSpec struct {
	Repo string `json:"repo"`
	URL  string `json:"url"`
}

// rawConfig mirrors the on-disk JSON. PortRange is deferred to json.RawMessage
// because it appears BOTH as an array [lo,hi] (real files) and as a legacy
// "lo hi" string (old default).
type rawConfig struct {
	Abbrev         string              `json:"abbrev"`
	BaseRef        string              `json:"baseRef"`
	WorktreeDir    string              `json:"worktreeDir"`
	PortEnv        string              `json:"portEnv"`
	Deps           string              `json:"deps"`
	DepsDir        string              `json:"depsDir"`
	Install        string              `json:"install"`
	User           string              `json:"user"`
	PortRange      json.RawMessage     `json:"portRange"`
	PortCount      int                 `json:"portCount"`
	Copy           []string            `json:"copy"`
	EnvFile        string              `json:"envFile"`
	HotfixBaseRef  string              `json:"hotfixBaseRef"`
	RedisPrefixEnv string              `json:"redisPrefixEnv"`
	Peers          map[string]PeerSpec `json:"peers"`
	Hotfix         rawHotfix           `json:"hotfix"`
	Gate           rawGate             `json:"gate"`
	Variants       map[string]Variant  `json:"variants"`
	DefaultVariant string              `json:"defaultVariant"`
}

// rawHotfix mirrors the on-disk "hotfix" stanza.
type rawHotfix struct {
	BaseStrategy string `json:"baseStrategy"`
}

// rawGate mirrors the on-disk "gate" stanza.
type rawGate struct {
	SharedStore    bool     `json:"sharedStore"`
	AccessPatterns []string `json:"accessPatterns"`
	DBAccessor     string   `json:"dbAccessor"`
}

// Load reads <repoRoot>/.claude/worktree.json and resolves defaults. A missing
// file is an error: wt cannot operate on a repo that does not declare one.
func Load(repoRoot string) (*Config, error) {
	path := filepath.Join(repoRoot, ".claude", "worktree.json")
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
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
		Abbrev:         orDefault(raw.Abbrev, filepath.Base(repoRoot)),
		BaseRef:        orDefault(raw.BaseRef, "origin/main"),
		WorktreeDir:    orDefault(raw.WorktreeDir, ".worktrees/"),
		PortEnv:        orDefault(raw.PortEnv, "PORT"),
		Deps:           orDefault(raw.Deps, "install"),
		DepsDir:        orDefault(raw.DepsDir, "node_modules"),
		Install:        raw.Install,
		User:           strings.TrimSpace(raw.User), // empty → resolved per-dev in New (BranchUser)
		PortRange:      pr,
		PortCount:      orInt(raw.PortCount, 1),
		Copy:           raw.Copy,
		EnvFile:        raw.EnvFile,
		HotfixBaseRef:  raw.HotfixBaseRef,
		RedisPrefixEnv: raw.RedisPrefixEnv,
		Peers:          raw.Peers,
	}
	c.HotfixBaseStrategy = orDefault(raw.Hotfix.BaseStrategy, "staging")
	c.GateSharedStore = raw.Gate.SharedStore
	c.GateAccessPatterns = raw.Gate.AccessPatterns
	c.GateDBAccessor = raw.Gate.DBAccessor
	c.Variants = raw.Variants
	c.DefaultVariant = strings.TrimSpace(raw.DefaultVariant)
	if err := validateVariants(c); err != nil {
		return nil, fmt.Errorf("wt: %s: %w", path, err)
	}
	return c, nil
}

// validateVariants rejects a defaultVariant that names no variant and any
// sparseExclude entry that is not a plain repo-relative path, so a bad config
// fails at load time instead of half-way through `wt new`.
func validateVariants(c *Config) error {
	if c.DefaultVariant != "" {
		if _, ok := c.Variants[c.DefaultVariant]; !ok {
			return fmt.Errorf("defaultVariant %q is not declared in variants", c.DefaultVariant)
		}
	}
	for name, v := range c.Variants {
		if !validSlug(name) {
			return fmt.Errorf("variant name %q must be lowercase letters, digits and dashes", name)
		}
		for _, p := range v.SparseExclude {
			if sparseExcludePattern(p) == "" {
				return fmt.Errorf("variant %q: sparseExclude entry %q must be a repo-relative path (no \"!\", \"..\", or glob)", name, p)
			}
		}
		for _, sm := range v.Submodules {
			sm = strings.TrimSpace(sm)
			if sm == "" || strings.HasPrefix(sm, "-") || filepath.IsAbs(sm) || strings.Contains(sm, "..") {
				return fmt.Errorf("variant %q: submodule path %q must be a repo-relative path", name, sm)
			}
		}
	}
	return nil
}

// sparseExcludePattern turns a repo-relative path ("android", "android/",
// "/ios/Podfile") into an anchored non-cone exclude pattern ("!/android",
// "!/ios/Podfile"). No trailing slash, so one pattern hides a file or a whole
// directory alike. It returns "" for anything that is not a plain path.
func sparseExcludePattern(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" || strings.ContainsAny(p, "!*?[]\\") || strings.HasPrefix(p, "-") {
		return ""
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return ""
		}
	}
	return "!/" + p
}

// ResolveVariant picks the variant `wt new` applies: the requested name, else
// defaultVariant, else none ("" with a zero Variant). A requested name the repo
// does not declare is an error that lists what it does declare.
func ResolveVariant(c *Config, requested string) (string, Variant, error) {
	name := strings.TrimSpace(requested)
	if name == "" {
		name = c.DefaultVariant
	}
	if name == "" {
		return "", Variant{}, nil
	}
	v, ok := c.Variants[name]
	if !ok {
		if len(c.Variants) == 0 {
			return "", Variant{}, fmt.Errorf("--variant %q: this repo declares no variants in .claude/worktree.json", name)
		}
		names := make([]string, 0, len(c.Variants))
		for n := range c.Variants {
			names = append(names, n)
		}
		sort.Strings(names)
		return "", Variant{}, fmt.Errorf("--variant %q is not declared; this repo has: %s", name, strings.Join(names, ", "))
	}
	return name, v, nil
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// orInt returns v when positive, else def. A missing JSON int is 0 → default.
func orInt(v, def int) int {
	if v > 0 {
		return v
	}
	return def
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

// HotfixBase resolves the ref a hotfix branches from. It is the single resolver
// behind both `wt new --type hotfix` and `zenify hotfix baseref`, so the two
// can never disagree. A declared hotfixBaseRef wins; "staging"/"standalone"
// (the default) follow baseRef. "release-latest" is retired: guessing the
// newest release<N> branch picks a stale one once a team stops cutting them.
func HotfixBase(c *Config) (string, error) {
	switch c.HotfixBaseStrategy {
	case "custom":
		if c.HotfixBaseRef == "" {
			return "", fmt.Errorf("hotfix.baseStrategy=custom but hotfixBaseRef is missing in .claude/worktree.json")
		}
		return c.HotfixBaseRef, nil
	case "release-latest":
		return "", fmt.Errorf(`hotfix.baseStrategy "release-latest" is retired: declare "hotfix": {"baseStrategy": "custom"} and "hotfixBaseRef": "<ref>" (e.g. "origin/main") in .claude/worktree.json`)
	case "staging", "standalone":
		if c.HotfixBaseRef != "" {
			return c.HotfixBaseRef, nil
		}
		return c.BaseRef, nil
	default:
		return "", fmt.Errorf("unknown hotfix.baseStrategy %q (want custom, staging or standalone)", c.HotfixBaseStrategy)
	}
}
