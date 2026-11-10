package uiverify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func evidenceText(t *testing.T, repo string, d Deps) (path, text string) {
	t.Helper()
	path, err := Evidence(d, repo, "origin/staging")
	if err != nil {
		t.Fatalf("Evidence: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, string(b)
}

func TestEvidence_Table(t *testing.T) {
	repo, d := gateRepo(t, "", "fpevid0001")
	writeArtifact(t, repo, "fpevid0001", Artifact{FP: "fpevid0001", Screens: []Screen{
		shotScreen("home", 1440, "pass"), shotScreen("home", 375, "pass"),
	}})
	path, text := evidenceText(t, repo, d)
	if want := filepath.Join(repo, ".znf", "ui-verify", "fpevid0001-evidence.md"); path != want {
		t.Errorf("path=%q want %q", path, want)
	}
	shot := filepath.Join(repo, ".znf", "ui-verify", "shot.png")
	for _, want := range []string{"fpevid0001", "| home | 1440 | pass | none | " + shot + " |", "| home | 375 | pass | none | " + shot + " |"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if !strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\n\n") {
		t.Errorf("want exactly one trailing newline: %q", text[len(text)-3:])
	}
}

func TestEvidence_NoArtifact(t *testing.T) {
	repo, d := gateRepo(t, "", "fpevid0002")
	_, text := evidenceText(t, repo, d)
	if !strings.Contains(text, "required") || !strings.Contains(text, "no artifact") {
		t.Errorf("want state and missing-artifact note:\n%s", text)
	}
}

func TestEvidence_StyleSection(t *testing.T) {
	repo, d := styleCheckDeps(t, "const c = '#ff0000'")
	_, text := evidenceText(t, repo, d)
	if !strings.Contains(text, "src/App.tsx:2 hex #ff0000") {
		t.Errorf("violation not listed:\n%s", text)
	}
}

func TestEvidence_StyleNotDeclared(t *testing.T) {
	repo, d := gateRepo(t, "", "fpevid0003")
	_, text := evidenceText(t, repo, d)
	if !strings.Contains(text, "not declared") {
		t.Errorf("want 'not declared':\n%s", text)
	}
}

func TestEvidence_VisualSection(t *testing.T) {
	repo, d := gateRepo(t, "", "fpevid0004")
	writeRepoFile(t, repo, ".znf/visual/routes.json", `[{"name":"home","path":"/"},{"name":"a","path":"/a"},{"name":"b","path":"/b"}]`)
	writeRepoFile(t, repo, ".znf/visual/__a11y__/a.json",
		`{"route":"a","violations":[{"id":"image-alt","impact":"critical","help":"x","nodes":1,"targets":["img"]},{"id":"c","impact":"serious","nodes":2}]}`)
	writeRepoFile(t, repo, ".znf/visual/__a11y__/b.json", `{broken`)
	writeRepoFile(t, repo, ".znf/visual/__diff__/x.png", "x")
	_, text := evidenceText(t, repo, d)
	for _, want := range []string{"home", "a: 2 violation(s) (1 critical, 1 serious)", "b: unreadable", filepath.Join(repo, ".znf", "visual", "__diff__")} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestEvidence_BadRoutesJSON(t *testing.T) {
	repo, d := gateRepo(t, "", "fpevid0005")
	writeRepoFile(t, repo, ".znf/visual/routes.json", `nope`)
	_, text := evidenceText(t, repo, d)
	if !strings.Contains(text, "unreadable:") {
		t.Errorf("want unreadable routes:\n%s", text)
	}
}

func TestEvidence_ScreenshotPathStaysInside(t *testing.T) {
	repo, d := gateRepo(t, "", "fpevid0006")
	bad := shotScreen("home", 1440, "pass")
	bad.Screenshot = "../../x"
	writeArtifact(t, repo, "fpevid0006", Artifact{FP: "fpevid0006", Screens: []Screen{bad}})
	_, text := evidenceText(t, repo, d)
	if !strings.Contains(text, "invalid entry") {
		t.Errorf("want invalid entry:\n%s", text)
	}
	if strings.Contains(text, "../") || strings.Contains(text, filepath.Join(repo, "x")) {
		t.Errorf("path outside artifact dir leaked:\n%s", text)
	}
}

func TestEvidence_BadConfig(t *testing.T) {
	repo, d := gateRepo(t, `{"uiVerify":{"viewport":[1]}}`, "fpevid0007")
	if _, err := Evidence(d, repo, "origin/staging"); exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("err=%v", err)
	}
}

// A report left behind by a removed route must not be shown as current.
func TestEvidence_A11yOnlyForListedRoutes(t *testing.T) {
	repo := t.TempDir()
	vdir := filepath.Join(repo, ".znf", "visual", "__a11y__")
	if err := os.MkdirAll(vdir, 0o750); err != nil {
		t.Fatal(err)
	}
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(repo, ".znf", "visual", "routes.json"), `[{"name":"home","path":"/"}]`)
	write(filepath.Join(vdir, "home.json"), `{"route":"home","violations":[]}`)
	write(filepath.Join(vdir, "gone.json"), `{"route":"gone","violations":[]}`)
	var b strings.Builder
	writeVisual(&b, repo)
	if out := b.String(); !strings.Contains(out, "- home:") || strings.Contains(out, "gone") {
		t.Errorf("visual section:\n%s", out)
	}
}
