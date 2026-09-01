package wt

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// WireOptions drives RunWire. WorktreePath is the worktree whose env file is
// rewritten; RepoRoot is that repo's main checkout (source of the baseline env).
type WireOptions struct {
	RepoRoot     string
	WorktreePath string
	DryRun       bool
	Runner       gitx.Runner
	Stdout       io.Writer
	Stderr       io.Writer
}

// RunWire points a worktree's env file at the peer services a task is changing.
// Drop-in port of bash cmd_wire: each declared var is recomputed from scratch —
// the peer's worktree port when a worktree with THIS slug exists in the peer
// repo, otherwise the value the main checkout's env file has. Idempotent and
// reversible: running it after a peer is torn down restores the baseline.
func RunWire(o WireOptions) error {
	if o.Stdout == nil {
		o.Stdout = io.Discard
	}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	r := o.Runner
	cfg, err := Load(o.RepoRoot)
	if err != nil {
		return err
	}
	slug := cfgGet(r, o.WorktreePath, "wt.slug")
	if slug == "" {
		return fmt.Errorf("wt: not inside a wt worktree — wire rewrites a worktree's env file, and the main checkout must keep its baseline")
	}
	if len(cfg.Peers) == 0 {
		fmt.Fprintf(o.Stdout, "wt: this repo declares no peers — nothing to wire\n")
		return nil
	}
	envFile := orEnvFile(cfg)
	wtEnv := filepath.Join(o.WorktreePath, envFile)
	mainEnv := filepath.Join(o.RepoRoot, envFile)
	workspace := filepath.Dir(o.RepoRoot)

	b, err := os.ReadFile(wtEnv)
	if err != nil {
		return fmt.Errorf("wt: no %s in this worktree — nothing to rewrite", envFile)
	}
	lines := strings.Split(string(b), "\n")
	baseline := ""
	if mb, e := os.ReadFile(mainEnv); e == nil {
		baseline = string(mb)
	}

	// Stable order: bash iterates JS Object insertion order; Go maps are random,
	// so sort the keys to keep output and the written file deterministic.
	names := make([]string, 0, len(cfg.Peers))
	for name := range cfg.Peers {
		names = append(names, name)
	}
	sort.Strings(names)

	changed := 0
	for _, name := range names {
		spec := cfg.Peers[name]
		var value, why string
		if wtp := peerWorktreePath(workspace, spec.Repo, slug); wtp != "" {
			port := cfgGet(r, wtp, "wt.port")
			if port == "" {
				fmt.Fprintf(o.Stderr, "wt:   %s — %s/%s has no wt.port recorded, left as is\n", name, spec.Repo, slug)
				continue
			}
			url := spec.URL
			if url == "" {
				url = "http://localhost:{port}"
			}
			value = strings.ReplaceAll(url, "{port}", port)
			why = fmt.Sprintf("%s worktree, port %s", spec.Repo, port)
		} else {
			v, ok := readVar(baseline, name)
			if !ok {
				fmt.Fprintf(o.Stderr, "wt:   %s — no worktree for %s and no baseline in the main %s, left as is\n", name, spec.Repo, envFile)
				continue
			}
			value = v
			why = "baseline from the main checkout"
		}
		current, has := readVar(strings.Join(lines, "\n"), name)
		if has && current == value {
			fmt.Fprintf(o.Stdout, "wt:   %s already %s (%s)\n", name, value, why)
			continue
		}
		if !has {
			lines = append(lines, name+"="+value)
		} else {
			for i, l := range lines {
				if strings.HasPrefix(l, name+"=") {
					lines[i] = name + "=" + value
				}
			}
		}
		fmt.Fprintf(o.Stdout, "wt:   %s=%s  (%s)\n", name, value, why)
		changed++
	}
	if !o.DryRun && changed > 0 {
		if err := os.WriteFile(wtEnv, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return fmt.Errorf("wt: write %s: %w", wtEnv, err)
		}
	}
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "wt: %d would change\n", changed)
	} else {
		fmt.Fprintf(o.Stdout, "wt: %d rewritten in %s\n", changed, envFile)
	}
	// The env file is expected gitignored. If rewriting dirtied a TRACKED file,
	// warn — wire must not be what commits a localhost URL.
	if !o.DryRun && changed > 0 {
		if d, _ := r.Run(o.WorktreePath, "status", "--porcelain", "--", envFile); strings.TrimSpace(string(d)) != "" {
			fmt.Fprintf(o.Stderr, "wt: WARNING — %s is TRACKED in this repo, so wiring dirtied the branch\n", envFile)
		}
	}
	return nil
}

// readVar returns the value of KEY=VALUE for name from env text (first match),
// matching bash readVar (a full-line value, newline excluded).
func readVar(text, name string) (string, bool) {
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, name+"=") {
			return l[len(name)+1:], true
		}
	}
	return "", false
}

// peerWorktreePath returns the peer repo's worktree dir for slug, or "" if it
// does not exist. The peer declares its own worktreeDir; fall back to
// ".worktrees/" when its worktree.json is absent or unreadable (bash parity).
func peerWorktreePath(workspace, repo, slug string) string {
	root := filepath.Join(workspace, repo)
	dir := ".worktrees/"
	if cb, err := os.ReadFile(filepath.Join(root, ".claude", "worktree.json")); err == nil {
		var pc struct {
			WorktreeDir string `json:"worktreeDir"`
		}
		if json.Unmarshal(cb, &pc) == nil && pc.WorktreeDir != "" {
			dir = pc.WorktreeDir
		}
	}
	p := filepath.Join(root, dir, slug)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}
