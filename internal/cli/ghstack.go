package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
)

const (
	ghStackMinVersion = "2.90.0"
	ghStackInstallCmd = "gh extension install github/gh-stack"
	ghStackExtension  = "github/gh-stack"
)

// versionLess reports whether dotted version a is numerically below b.
func versionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var na, nb int
		if i < len(pa) {
			na, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			nb, _ = strconv.Atoi(pb[i])
		}
		if na != nb {
			return na < nb
		}
	}
	return false
}

// ghStackStatus reports whether gh is new enough and the gh-stack extension is installed.
func ghStackStatus(gh ghx.Runner) (bool, string) {
	out, err := gh.Run("--version")
	if err != nil {
		return false, "gh not available"
	}
	first, _, _ := strings.Cut(string(out), "\n")
	f := strings.Fields(first) // gh version 2.100.0 (2026-09-03)
	if len(f) < 3 || f[0] != "gh" || f[1] != "version" {
		return false, "gh not available"
	}
	ver := f[2]
	if versionLess(ver, ghStackMinVersion) {
		return false, fmt.Sprintf("gh %s < %s — upgrade gh", ver, ghStackMinVersion)
	}
	list, err := gh.Run("extension", "list")
	if err != nil {
		return false, "gh not available"
	}
	for _, line := range strings.Split(string(list), "\n") {
		cols := strings.Split(line, "\t")
		if len(cols) >= 2 && strings.TrimSpace(cols[1]) == ghStackExtension {
			extVer := ""
			if len(cols) >= 3 {
				extVer = strings.TrimSpace(cols[2])
			}
			return true, fmt.Sprintf("gh %s, gh-stack %s", ver, extVer)
		}
	}
	return false, "gh-stack extension missing — run " + ghStackInstallCmd
}

// installGHStack installs the extension; it returns the install error, if any.
func installGHStack(gh ghx.Runner) error {
	_, err := gh.Run(strings.Fields(ghStackInstallCmd)[1:]...)
	return err
}

// ensureGHStack installs the gh-stack extension when missing. Never fatal.
func ensureGHStack(gh ghx.Runner, w, errW io.Writer) {
	ok, detail := ghStackStatus(gh)
	if ok {
		return
	}
	if !strings.Contains(detail, "extension missing") {
		fmt.Fprintf(errW, "warning: gh-stack: %s\n", detail)
		return
	}
	if err := installGHStack(gh); err != nil {
		fmt.Fprintf(errW, "warning: gh-stack install: %v\n", err)
		return
	}
	fmt.Fprintln(w, "gh-stack: installed")
}
