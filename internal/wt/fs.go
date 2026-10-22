package wt

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// SeedCopyFiles copies each still-present copy-array target from repoRoot into
// worktreePath, creating parent dirs. A declared target that does not exist in
// the repo is NOT an error (matching bash wt) — it is returned as a warning
// string. Uses `cp -c -R` (copy-on-write where the FS allows) with a plain
// `cp -R` fallback, so large trees stay cheap.
func SeedCopyFiles(repoRoot, worktreePath string, copyList []string) ([]string, error) {
	var warns []string
	for _, f := range copyList {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		src := filepath.Join(repoRoot, f)
		if _, err := os.Stat(src); err != nil {
			warns = append(warns, fmt.Sprintf("declared copy target %q does not exist in the repo root", f))
			continue
		}
		dst := filepath.Join(worktreePath, f)
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return warns, fmt.Errorf("wt: parent dir for copy target %q: %w", f, err)
		}
		if err := copyTree(src, dst); err != nil {
			return warns, fmt.Errorf("wt: seed copy target %q: %w", f, err)
		}
	}
	return warns, nil
}

// copyTree copies src→dst preferring APFS copy-on-write (`cp -c -R`), falling
// back to a plain recursive copy. Shelling to cp keeps CoW that a Go copy loses.
//
// The first attempt can fail partway through a large tree (ENOSPC, a file
// vanishing mid-walk, a permission on one entry) and leave dst half-populated.
// Because `cp -R src dst` copies INTO dst when dst already exists — producing a
// nested dst/<basename(src)> instead of populating dst — the fallback MUST start
// from the same clean precondition as the first attempt, so remove any partial
// dst first. cp's own stderr is captured into the returned error: without it a
// real failure (disk full, cross-device) surfaces only as "exit status 1".
func copyTree(src, dst string) error {
	if err := exec.Command("cp", "-c", "-R", src, dst).Run(); err == nil { //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
		return nil
	}
	// Clear whatever the failed CoW attempt may have left, so the fallback does
	// not nest into an existing dst.
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("clear partial copy at %s: %w", dst, err)
	}
	var stderr strings.Builder
	cmd := exec.Command("cp", "-R", src, dst) //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

// upsertEnvVar sets key=value in envPath: replacing an existing line for that key,
// or appending one (guaranteeing a newline before the appended line). A missing
// file is created with just the single line. This is the replace-or-append core
// WritePortEnv previously inlined.
func upsertEnvVar(envPath, key, value string) error {
	line := key + "=" + value
	b, err := os.ReadFile(envPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if os.IsNotExist(err) {
		return os.WriteFile(envPath, []byte(line+"\n"), 0o600)
	}
	if err != nil {
		return err
	}
	content := string(b)
	lines := strings.Split(content, "\n")
	replaced := false
	for i, l := range lines {
		if strings.HasPrefix(l, key+"=") {
			lines[i] = line
			replaced = true
			break
		}
	}
	if replaced {
		return os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0o600) //nolint:gosec // G703 -- envPath is the worktree's own env file, computed internally from the repo's configured worktree path, not externally-tainted input
	}
	// append, ensuring exactly one separating newline
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += line + "\n"
	return os.WriteFile(envPath, []byte(content), 0o600) //nolint:gosec // G703 -- envPath is the worktree's own env file, computed internally from the repo's configured worktree path, not externally-tainted input
}

// WritePortEnv sets <portEnv>=<port> in envPath: replacing an existing line for
// that key, or appending one (guaranteeing a newline before the appended line).
// A missing file is created with just the single line.
func WritePortEnv(envPath, portEnv string, port int) error {
	return upsertEnvVar(envPath, portEnv, strconv.Itoa(port))
}

// SeedIdentityEnv writes the FR-038 identity variables into the worktree env file
// so parallel worktrees of one repo do not collide on the shared, stateful infra
// that a per-worktree port cannot isolate (Docker Compose project, Redis/BullMQ
// key space): WT_SLUG, COMPOSE_PROJECT_NAME=<abbrev>-<slug>, and — only when the
// config names the env var to hold it — a Redis key-prefix <abbrev>-<slug>:.
func SeedIdentityEnv(envPath string, cfg *Config, slug string) error {
	compose := slug
	if cfg.Abbrev != "" {
		compose = cfg.Abbrev + "-" + slug
	}
	if err := upsertEnvVar(envPath, "WT_SLUG", slug); err != nil {
		return err
	}
	if err := upsertEnvVar(envPath, "COMPOSE_PROJECT_NAME", compose); err != nil {
		return err
	}
	if cfg.RedisPrefixEnv != "" {
		if err := upsertEnvVar(envPath, cfg.RedisPrefixEnv, compose+":"); err != nil {
			return err
		}
	}
	return nil
}

// ApplyDeps sets up the dependency dir (e.g. node_modules) in the worktree per
// mode: "symlink" links to the main checkout's dir; "clone" CoW-copies it then
// runs install; "install" just runs install. symlink/clone require the source
// dir to exist in repoRoot. installCmd empty → install step is skipped.
//
// When ZENIFY_DEPS_CACHE is set and holds <repo>/<depsDirName>, symlink/clone
// seed from that cache if its package-lock.json matches the worktree's; on a
// lock mismatch the cache is ignored and installCmd runs instead.
func ApplyDeps(repoRoot, worktreePath, mode, depsDirName, installCmd string) error {
	dst := filepath.Join(worktreePath, depsDirName)
	src := filepath.Join(repoRoot, depsDirName)
	if mode == "symlink" || mode == "clone" {
		cached, _ := depsSource(repoRoot, worktreePath, depsDirName, os.Getenv)
		if cached == "" {
			return runInstall(worktreePath, installCmd)
		}
		src = cached
	}
	switch mode {
	case "symlink":
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("wt: deps=symlink but %s does not exist — install in the main checkout first", src)
		}
		return os.Symlink(src, dst)
	case "clone":
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("wt: deps=clone but %s does not exist — install in the main checkout first", src)
		}
		if err := copyTree(src, dst); err != nil {
			return fmt.Errorf("wt: clone %s: %w", depsDirName, err)
		}
		return runInstall(worktreePath, installCmd)
	case "install":
		return runInstall(worktreePath, installCmd)
	case "none":
		// Repo uses a global dependency cache (e.g. Maven ~/.m2): there is no
		// per-project deps dir to clone/symlink, and no per-worktree install step.
		return nil
	default:
		return fmt.Errorf("wt: unknown deps mode: %s", mode)
	}
}

// depsSource picks where symlink/clone read the deps dir from. With
// ZENIFY_DEPS_CACHE unset, or no <cache>/<repo>/<depsDirName> in it, it is the
// repo root's dir (fromCache=false). If the cache entry exists and its
// package-lock.json is byte-equal to the worktree's, it is the cache dir
// (fromCache=true). If the entry exists but the lock differs (or is unreadable)
// it returns "" so the caller installs instead of using stale deps.
func depsSource(repoRoot, worktreePath, depsDirName string, getenv func(string) string) (src string, fromCache bool) {
	own := filepath.Join(repoRoot, depsDirName)
	cache := getenv("ZENIFY_DEPS_CACHE")
	if cache == "" {
		return own, false
	}
	entry := filepath.Join(cache, filepath.Base(repoRoot))
	cached := filepath.Join(entry, depsDirName)
	if fi, err := os.Stat(cached); err != nil || !fi.IsDir() {
		return own, false
	}
	a, errA := os.ReadFile(filepath.Join(entry, "package-lock.json"))        //nolint:gosec // G304 -- path built from operator-set cache dir and repo basename
	b, errB := os.ReadFile(filepath.Join(worktreePath, "package-lock.json")) //nolint:gosec // G304 -- path is the worktree this tool created
	if errA != nil || errB != nil || !bytes.Equal(a, b) {
		return "", false
	}
	return cached, true
}

// runInstall runs installCmd inside dir via the shell. An empty command is a
// no-op (matching bash wt's run_install, which skips when none is declared).
func runInstall(dir, installCmd string) error {
	if strings.TrimSpace(installCmd) == "" {
		return nil
	}
	cmd := exec.Command("sh", "-c", installCmd) //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
	cmd.Dir = dir
	cmd.Stdout = os.Stderr // install chatter goes to stderr, never stdout (path contract)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
