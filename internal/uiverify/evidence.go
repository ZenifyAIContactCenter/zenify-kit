package uiverify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// Evidence writes <artifactDir>/<fp>-evidence.md and returns its absolute path. It reports what is
// on disk and never gates: only a bad config (BadArgs) or an unwritable file is an error.
func Evidence(d Deps, repo, base string) (string, error) {
	abs, err := filepath.Abs(repo)
	if err != nil {
		return "", exitcode.New(exitcode.Fail, err)
	}
	repo = abs
	// Check's "required"/"style" errors are findings here; only an empty state is a real failure.
	state, msg, err := Check(d, repo, base)
	if err != nil && state == "" {
		return "", err
	}
	fp, err := Fingerprint(d, repo)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# UI verify evidence\n\n**Fingerprint:** %s\n\n**Generated:** %s\n\n", fp, d.now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "## Check\n\n**State:** %s\n\n**Message:** %s\n\n", state, oneLine(msg))
	writeScreens(&b, repo, fp)
	writeStyle(&b, d, repo, base)
	writeVisual(&b, repo)

	dir := artifactDir(repo)
	if err := ensureArtifactDir(dir); err != nil {
		return "", exitcode.New(exitcode.Fail, err)
	}
	path := filepath.Join(dir, fp+"-evidence.md")
	out := strings.TrimRight(b.String(), "\n") + "\n"
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		return "", exitcode.New(exitcode.Fail, err)
	}
	return path, nil
}

// oneLine keeps untrusted text from breaking a table row or a list item.
func oneLine(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ", "|", `\|`).Replace(s)
}

func writeScreens(b *strings.Builder, repo, fp string) {
	b.WriteString("## Screens\n\n")
	dir := artifactDir(repo)
	raw, err := os.ReadFile(filepath.Join(dir, fp+".json")) //nolint:gosec // G304 -- path built from fp, not user input
	if err != nil {
		b.WriteString("no artifact for this fingerprint\n\n")
		return
	}
	var art Artifact
	if err := json.Unmarshal(raw, &art); err != nil {
		fmt.Fprintf(b, "unreadable artifact: %s\n\n", oneLine(err.Error()))
		return
	}
	b.WriteString("| Screen | Viewport | Verdict | Overflow | Screenshot |\n|---|---|---|---|---|\n")
	for _, s := range art.Screens {
		if !validEntry(s) {
			fmt.Fprintf(b, "| %s | %d | invalid entry | | |\n", oneLine(s.Screen), s.Viewport)
			continue
		}
		ov := Overflows(s.Layout)
		verdict, detail := "pass", "none"
		if reason := strings.TrimSpace(s.FailReason); len(ov) > 0 || reason != "" {
			verdict, detail = "fail", oneLine(failDetail(ov, reason))
		}
		fmt.Fprintf(b, "| %s | %d | %s | %s | %s |\n", oneLine(s.Screen), s.Viewport, verdict, detail,
			oneLine(filepath.Join(dir, s.Screenshot)))
	}
	b.WriteString("\n")
}

func writeStyle(b *strings.Builder, d Deps, repo, base string) {
	b.WriteString("## Style\n\n")
	vs, checked, declared, err := Style(d, repo, base)
	switch {
	case err != nil:
		fmt.Fprintf(b, "unreadable: %s\n\n", oneLine(err.Error()))
	case !declared:
		b.WriteString("not declared\n\n")
	case len(vs) == 0:
		fmt.Fprintf(b, "clean (%d added lines checked)\n\n", checked)
	default:
		fmt.Fprintf(b, "%d violation(s)\n\n", len(vs))
		for _, v := range vs {
			fmt.Fprintf(b, "- %s\n", oneLine(fmt.Sprintf("%s:%d %s %s", v.File, v.Line, v.Rule, v.Match)))
		}
		b.WriteString("\n")
	}
}

type a11yFile struct {
	Violations []struct {
		Impact string `json:"impact"`
	} `json:"violations"`
}

// writeVisual reads .znf/visual by hand rather than importing internal/visual (docker deps).
func writeVisual(b *strings.Builder, repo string) {
	b.WriteString("## Visual\n\n")
	vdir := filepath.Join(repo, ".znf", "visual")

	b.WriteString("**Routes:** ")
	var routes []struct {
		Name string `json:"name"`
	}
	if raw, err := os.ReadFile(filepath.Join(vdir, "routes.json")); err != nil { //nolint:gosec // G304 -- fixed path under --repo
		b.WriteString("none\n\n")
	} else if err := json.Unmarshal(raw, &routes); err != nil {
		fmt.Fprintf(b, "unreadable: %s\n\n", oneLine(err.Error()))
	} else {
		names := make([]string, len(routes))
		for i, r := range routes {
			names[i] = oneLine(r.Name)
		}
		fmt.Fprintf(b, "%s\n\n", strings.Join(names, ", "))
	}

	b.WriteString("**A11y (advisory):**\n\n")
	files, _ := filepath.Glob(filepath.Join(vdir, "__a11y__", "*.json"))
	sort.Strings(files)
	if len(files) == 0 {
		b.WriteString("- not run\n")
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		var a a11yFile
		raw, err := os.ReadFile(f) //nolint:gosec // G304 -- glob result under --repo
		if err == nil {
			err = json.Unmarshal(raw, &a)
		}
		if err != nil {
			fmt.Fprintf(b, "- %s: unreadable: %s\n", oneLine(name), oneLine(err.Error()))
			continue
		}
		crit, ser := 0, 0
		for _, v := range a.Violations {
			switch v.Impact {
			case "critical":
				crit++
			case "serious":
				ser++
			}
		}
		fmt.Fprintf(b, "- %s: %d violation(s) (%d critical, %d serious)\n", oneLine(name), len(a.Violations), crit, ser)
	}

	if st, err := os.Stat(filepath.Join(vdir, "__diff__")); err == nil && st.IsDir() {
		fmt.Fprintf(b, "\n**Diff images:** %s\n", filepath.Join(vdir, "__diff__"))
	}
}
