package wt

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
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
func copyTree(src, dst string) error {
	if err := exec.Command("cp", "-c", "-R", src, dst).Run(); err == nil {
		return nil
	}
	return exec.Command("cp", "-R", src, dst).Run()
}

// WritePortEnv sets <portEnv>=<port> in envPath: replacing an existing line for
// that key, or appending one (guaranteeing a newline before the appended line).
// A missing file is created with just the single line.
func WritePortEnv(envPath, portEnv string, port int) error {
	line := fmt.Sprintf("%s=%d", portEnv, port)
	b, err := os.ReadFile(envPath)
	if os.IsNotExist(err) {
		return os.WriteFile(envPath, []byte(line+"\n"), 0o644)
	}
	if err != nil {
		return err
	}
	content := string(b)
	lines := strings.Split(content, "\n")
	replaced := false
	for i, l := range lines {
		if strings.HasPrefix(l, portEnv+"=") {
			lines[i] = line
			replaced = true
			break
		}
	}
	if replaced {
		return os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0o644)
	}
	// append, ensuring exactly one separating newline
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += line + "\n"
	return os.WriteFile(envPath, []byte(content), 0o644)
}

// ApplyDeps sets up the dependency dir (e.g. node_modules) in the worktree per
// mode: "symlink" links to the main checkout's dir; "clone" CoW-copies it then
// runs install; "install" just runs install. symlink/clone require the source
// dir to exist in repoRoot. installCmd empty → install step is skipped.
func ApplyDeps(repoRoot, worktreePath, mode, depsDirName, installCmd string) error {
	src := filepath.Join(repoRoot, depsDirName)
	dst := filepath.Join(worktreePath, depsDirName)
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
	default:
		return fmt.Errorf("wt: unknown deps mode: %s", mode)
	}
}

// runInstall runs installCmd inside dir via the shell. An empty command is a
// no-op (matching bash wt's run_install, which skips when none is declared).
func runInstall(dir, installCmd string) error {
	if strings.TrimSpace(installCmd) == "" {
		return nil
	}
	cmd := exec.Command("sh", "-c", installCmd)
	cmd.Dir = dir
	cmd.Stdout = os.Stderr // install chatter goes to stderr, never stdout (path contract)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
