package uiverify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// fakeRunGit builds a Deps.RunGit that returns canned output keyed by the
// joined args, so tests never depend on a real git binary.
func fakeRunGit(t *testing.T, responses map[string]string) func(dir string, args ...string) (string, error) {
	t.Helper()
	return func(_ string, args ...string) (string, error) {
		key := strings.Join(args, " ")
		if out, ok := responses[key]; ok {
			return out, nil
		}
		// hash-object --stdin carries its content as the trailing arg
		// (see Fingerprint's doc comment) — match on the prefix only.
		if len(args) >= 2 && args[0] == "hash-object" && args[1] == "--stdin" {
			if out, ok := responses["hash-object --stdin"]; ok {
				return out, nil
			}
		}
		t.Fatalf("unexpected git call: %v", args)
		return "", nil
	}
}

func TestRenderSet_PathTable(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"src/App.tsx", true},
		{"main.go", false},
		{"src/pages/x.ts", true},
		{"x.css", true},
		{"internal/uiverify/uiverify.go", false},
		{"src/components/Button.jsx", true},
	}
	var diffNames []string
	for _, c := range cases {
		diffNames = append(diffNames, c.path)
	}
	runGit := fakeRunGit(t, map[string]string{
		"diff --name-only origin/staging..HEAD": strings.Join(diffNames, "\n"),
		"status --porcelain -uall":              "",
	})
	got, err := RenderSet(Deps{RunGit: runGit}, "/repo", "origin/staging")
	if err != nil {
		t.Fatal(err)
	}
	gotSet := map[string]bool{}
	for _, p := range got {
		gotSet[p] = true
	}
	for _, c := range cases {
		if gotSet[c.path] != c.want {
			t.Errorf("path %q: got %v, want %v (renderSet=%v)", c.path, gotSet[c.path], c.want, got)
		}
	}
}

func TestRenderSet_UnionsWorkingTree(t *testing.T) {
	runGit := fakeRunGit(t, map[string]string{
		"diff --name-only origin/staging..HEAD": "main.go",
		"status --porcelain -uall":              "?? src/pages/new.tsx\n M src/components/Old.jsx\n",
	})
	got, err := RenderSet(Deps{RunGit: runGit}, "/repo", "origin/staging")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"src/components/Old.jsx", "src/pages/new.tsx"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestWaiver(t *testing.T) {
	runGit := fakeRunGit(t, map[string]string{
		"diff origin/staging..HEAD": "some diff\nznf:ui-verify-ok: layout unchanged, only copy\n",
		"diff HEAD":                 "",
	})
	reason, ok := Waiver(Deps{RunGit: runGit}, "/repo", "origin/staging")
	if !ok {
		t.Fatal("expected waiver found")
	}
	if reason != "layout unchanged, only copy" {
		t.Errorf("reason = %q", reason)
	}

	runGitNone := fakeRunGit(t, map[string]string{
		"diff origin/staging..HEAD": "some diff\nno marker here\n",
		"diff HEAD":                 "still nothing\n",
	})
	if _, ok := Waiver(Deps{RunGit: runGitNone}, "/repo", "origin/staging"); ok {
		t.Fatal("expected no waiver")
	}
}

func TestWaiver_FoundInWorkingTree(t *testing.T) {
	runGit := fakeRunGit(t, map[string]string{
		"diff origin/staging..HEAD": "committed diff, no marker\n",
		"diff HEAD":                 "uncommitted\nznf:ui-verify-ok: wip\n",
	})
	reason, ok := Waiver(Deps{RunGit: runGit}, "/repo", "origin/staging")
	if !ok || reason != "wip" {
		t.Errorf("got reason=%q ok=%v", reason, ok)
	}
}

func writeTempScreenshot(t *testing.T, dir, name string, size int) string {
	t.Helper()
	p := filepath.Join(dir, name)
	data := make([]byte, size)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func stubDeps(fp string) Deps {
	return Deps{
		RunGit: func(_ string, args ...string) (string, error) {
			// Any git call for Fingerprint's purposes returns a fixed
			// hash-object output; other calls return empty.
			if len(args) >= 1 && args[0] == "hash-object" {
				return fp, nil
			}
			return "x", nil
		},
		Now: func() time.Time { return time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC) },
	}
}

func TestRecord_Success(t *testing.T) {
	repo := t.TempDir()
	src := writeTempScreenshot(t, t.TempDir(), "shot.png", 128)
	d := stubDeps("abc1234567")

	s := Screen{Screen: "home", Verdict: "pass", Measurement: Measurement{ChildRight: 1, ContainerRight: 2, PaddingRight: 3}}
	if err := Record(d, repo, s, src); err != nil {
		t.Fatal(err)
	}

	destPath := filepath.Join(repo, ".znf", "ui-verify", "abc1234567-home.png")
	if info, err := os.Stat(destPath); err != nil || info.Size() != 128 {
		t.Fatalf("screenshot not copied to %s: %v", destPath, err)
	}
	giPath := filepath.Join(repo, ".znf", "ui-verify", ".gitignore")
	if b, err := os.ReadFile(giPath); err != nil || string(b) != "*\n" {
		t.Fatalf(".gitignore = %q, err=%v", b, err)
	}

	artPath := filepath.Join(repo, ".znf", "ui-verify", "abc1234567.json")
	b, err := os.ReadFile(artPath)
	if err != nil {
		t.Fatal(err)
	}
	var art Artifact
	if err := json.Unmarshal(b, &art); err != nil {
		t.Fatal(err)
	}
	if len(art.Screens) != 1 || art.Screens[0].Screen != "home" {
		t.Fatalf("artifact = %+v", art)
	}
	if art.Screens[0].Screenshot != "abc1234567-home.png" {
		t.Errorf("screenshot field = %q", art.Screens[0].Screenshot)
	}
}

func TestRecord_Reject0Byte(t *testing.T) {
	repo := t.TempDir()
	src := writeTempScreenshot(t, t.TempDir(), "empty.png", 0)
	d := stubDeps("abc1234567")

	err := Record(d, repo, Screen{Screen: "home"}, src)
	if err == nil {
		t.Fatal("expected error for 0-byte screenshot")
	}
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("code = %d, want BadArgs", exitcode.Code(err))
	}
	artPath := filepath.Join(repo, ".znf", "ui-verify", "abc1234567.json")
	if _, err := os.Stat(artPath); !os.IsNotExist(err) {
		t.Errorf("artifact json should not be created, err=%v", err)
	}
}

func TestRecord_RejectMissingSrc(t *testing.T) {
	repo := t.TempDir()
	d := stubDeps("abc1234567")
	err := Record(d, repo, Screen{Screen: "home"}, filepath.Join(t.TempDir(), "nope.png"))
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("code = %d, want BadArgs", exitcode.Code(err))
	}
}

func TestRecord_Upsert(t *testing.T) {
	repo := t.TempDir()
	srcDir := t.TempDir()
	d := stubDeps("abc1234567")

	src1 := writeTempScreenshot(t, srcDir, "s1.png", 64)
	if err := Record(d, repo, Screen{Screen: "home", Verdict: "pass"}, src1); err != nil {
		t.Fatal(err)
	}
	src2 := writeTempScreenshot(t, srcDir, "s2.png", 64)
	if err := Record(d, repo, Screen{Screen: "detail", Verdict: "pass"}, src2); err != nil {
		t.Fatal(err)
	}

	artPath := filepath.Join(repo, ".znf", "ui-verify", "abc1234567.json")
	readArt := func() Artifact {
		b, err := os.ReadFile(artPath)
		if err != nil {
			t.Fatal(err)
		}
		var art Artifact
		if err := json.Unmarshal(b, &art); err != nil {
			t.Fatal(err)
		}
		return art
	}
	art := readArt()
	if len(art.Screens) != 2 {
		t.Fatalf("expected 2 screens, got %d: %+v", len(art.Screens), art.Screens)
	}

	// Re-record "home" with a different verdict — still 2 entries, updated.
	src3 := writeTempScreenshot(t, srcDir, "s3.png", 64)
	if err := Record(d, repo, Screen{Screen: "home", Verdict: "fail"}, src3); err != nil {
		t.Fatal(err)
	}
	art = readArt()
	if len(art.Screens) != 2 {
		t.Fatalf("expected still 2 screens after upsert, got %d: %+v", len(art.Screens), art.Screens)
	}
	var homeVerdict string
	for _, s := range art.Screens {
		if s.Screen == "home" {
			homeVerdict = s.Verdict
		}
	}
	if homeVerdict != "fail" {
		t.Errorf("home verdict = %q, want fail (upsert should replace)", homeVerdict)
	}
}

func writeArtifact(t *testing.T, repo, fp string, art Artifact) {
	t.Helper()
	dir := filepath.Join(repo, ".znf", "ui-verify")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(art)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fp+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestValidate_Ok(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".znf", "ui-verify")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shot.png"), []byte{1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, repo, "fp1", Artifact{
		FP:      "fp1",
		Screens: []Screen{{Screen: "home", Screenshot: "shot.png"}},
	})
	n, ok := Validate(repo, "fp1")
	if !ok || n != 1 {
		t.Errorf("n=%d ok=%v, want 1 true", n, ok)
	}
}

func TestValidate_MissingScreenshot(t *testing.T) {
	repo := t.TempDir()
	writeArtifact(t, repo, "fp1", Artifact{
		FP:      "fp1",
		Screens: []Screen{{Screen: "home", Screenshot: "missing.png"}},
	})
	_, ok := Validate(repo, "fp1")
	if ok {
		t.Error("expected !ok when screenshot file is missing")
	}
}

func TestValidate_MissingJSON(t *testing.T) {
	repo := t.TempDir()
	n, ok := Validate(repo, "nope")
	if ok || n != 0 {
		t.Errorf("n=%d ok=%v, want 0 false", n, ok)
	}
}

// checkDeps builds Deps whose RunGit answers RenderSet/Waiver/Fingerprint
// calls from a fixed table, keyed by joined args.
func checkDeps(t *testing.T, table map[string]string, fp string) Deps {
	t.Helper()
	return Deps{
		RunGit: func(_ string, args ...string) (string, error) {
			key := strings.Join(args, " ")
			if out, ok := table[key]; ok {
				return out, nil
			}
			if len(args) >= 2 && args[0] == "hash-object" && args[1] == "--stdin" {
				return fp, nil
			}
			return "", nil
		},
	}
}

func TestCheck_SC1_NotRequired(t *testing.T) {
	repo := t.TempDir()
	d := checkDeps(t, map[string]string{
		"diff --name-only origin/staging..HEAD": "main.go\nREADME.md",
		"status --porcelain -uall":              "",
	}, "fp000")
	state, _, err := Check(d, repo, "origin/staging")
	if state != "not_required" || err != nil {
		t.Errorf("state=%q err=%v", state, err)
	}
}

func TestCheck_SC2_Waived(t *testing.T) {
	repo := t.TempDir()
	d := checkDeps(t, map[string]string{
		"diff --name-only origin/staging..HEAD": "src/App.tsx",
		"status --porcelain -uall":              "",
		"diff origin/staging..HEAD":             "znf:ui-verify-ok: reviewed manually",
		"diff HEAD":                             "",
	}, "fp000")
	state, msg, err := Check(d, repo, "origin/staging")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(state, "waived:") || !strings.Contains(msg, "reviewed manually") {
		t.Errorf("state=%q msg=%q", state, msg)
	}
}

func TestCheck_SC3_Verified(t *testing.T) {
	repo := t.TempDir()
	fp := "fpverified"
	dir := filepath.Join(repo, ".znf", "ui-verify")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shot.png"), []byte{1}, 0o600); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, repo, fp, Artifact{FP: fp, Screens: []Screen{{Screen: "home", Screenshot: "shot.png"}}})

	d := checkDeps(t, map[string]string{
		"diff --name-only origin/staging..HEAD": "src/App.tsx",
		"status --porcelain -uall":              "",
		"diff origin/staging..HEAD":             "",
		"diff HEAD":                             "",
	}, fp)
	state, _, err := Check(d, repo, "origin/staging")
	if state != "verified" || err != nil {
		t.Errorf("state=%q err=%v", state, err)
	}
}

func TestCheck_SC4_RequiredMissingArtifact(t *testing.T) {
	repo := t.TempDir()
	d := checkDeps(t, map[string]string{
		"diff --name-only origin/staging..HEAD": "src/App.tsx",
		"status --porcelain -uall":              "",
		"diff origin/staging..HEAD":             "",
		"diff HEAD":                             "",
	}, "fpmissing")
	state, _, err := Check(d, repo, "origin/staging")
	if state != "required" {
		t.Errorf("state=%q, want required", state)
	}
	if exitcode.Code(err) != exitcode.Fail {
		t.Errorf("code=%d, want Fail", exitcode.Code(err))
	}
}

func TestCheck_SC5_RequiredInvalidArtifact(t *testing.T) {
	repo := t.TempDir()
	fp := "fpinvalid1"
	writeArtifact(t, repo, fp, Artifact{FP: fp, Screens: []Screen{{Screen: "home", Screenshot: "missing.png"}}})

	d := checkDeps(t, map[string]string{
		"diff --name-only origin/staging..HEAD": "src/App.tsx",
		"status --porcelain -uall":              "",
		"diff origin/staging..HEAD":             "",
		"diff HEAD":                             "",
	}, fp)
	state, _, err := Check(d, repo, "origin/staging")
	if state != "required" || exitcode.Code(err) != exitcode.Fail {
		t.Errorf("state=%q err=%v", state, err)
	}
}

func TestCheck_SC6_FPMismatchIsRequired(t *testing.T) {
	repo := t.TempDir()
	// A valid artifact exists, but for a DIFFERENT fp than what Fingerprint
	// now computes (working tree changed since the artifact was recorded).
	oldFP := "fpold000001"
	dir := filepath.Join(repo, ".znf", "ui-verify")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shot.png"), []byte{1}, 0o600); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, repo, oldFP, Artifact{FP: oldFP, Screens: []Screen{{Screen: "home", Screenshot: "shot.png"}}})

	newFP := "fpnew000002"
	d := checkDeps(t, map[string]string{
		"diff --name-only origin/staging..HEAD": "src/App.tsx",
		"status --porcelain -uall":              "",
		"diff origin/staging..HEAD":             "",
		"diff HEAD":                             "",
	}, newFP)
	state, _, err := Check(d, repo, "origin/staging")
	if state != "required" || exitcode.Code(err) != exitcode.Fail {
		t.Errorf("state=%q err=%v, want required/Fail on fp mismatch", state, err)
	}
}

func TestFingerprint_ConcatenatesAndTruncates(t *testing.T) {
	var gotHashInput string
	runGit := func(_ string, args ...string) (string, error) {
		switch {
		case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "HEAD":
			return "headsha\n", nil
		case len(args) >= 2 && args[0] == "diff" && args[1] == "HEAD":
			return "diffcontent\n", nil
		case len(args) >= 1 && args[0] == "status":
			return "statuscontent\n", nil
		case len(args) >= 2 && args[0] == "hash-object" && args[1] == "--stdin":
			gotHashInput = args[2]
			return "0123456789abcdef", nil
		}
		t.Fatalf("unexpected call: %v", args)
		return "", nil
	}
	fp, err := Fingerprint(Deps{RunGit: runGit}, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if fp != "0123456789" {
		t.Errorf("fp = %q, want first 10 chars", fp)
	}
	want := "headsha\n" + "diffcontent\n" + "statuscontent\n"
	if gotHashInput != want {
		t.Errorf("hash input = %q, want %q", gotHashInput, want)
	}
}
