package uiverify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
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
		// Test files render nothing a person sees, even under a UI directory.
		{"hooks/config.test.tsx", false},
		{"src/components/Button.test.jsx", false},
		{"src/pages/list.spec.tsx", false},
		{"src/components/__tests__/Button.jsx", false},
		{"src/components/testing/Grid.tsx", true},
		// Segment names only count as a whole path segment.
		{"internal/plugin/assets/znf/skills/review/SKILL.md", false},
		{"src/views/Home.js", true},
		{"views/a.js", true},
		{"previews/a.js", false},
	}
	var diffNames []string
	for _, c := range cases {
		diffNames = append(diffNames, c.path)
	}
	runGit := fakeRunGit(t, map[string]string{
		"-c core.quotePath=false diff --name-only origin/staging...HEAD": strings.Join(diffNames, "\n"),
		"-c core.quotePath=false status --porcelain -uall":              "",
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
		"-c core.quotePath=false diff --name-only origin/staging...HEAD": "main.go",
		"-c core.quotePath=false status --porcelain -uall":              "?? src/pages/new.tsx\n M src/components/Old.jsx\n",
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

// measureJSON builds a measurement: container right 100 with padding-right 4
// (content right 96), child right as given.
func measureJSON(innerWidth, childRight string) string {
	return `{"innerWidth":` + innerWidth + `,"child":{"left":0,"top":0,"right":` + childRight +
		`,"bottom":10},"container":{"left":0,"top":0,"right":100,"bottom":50},` +
		`"padding":{"left":0,"top":0,"right":4,"bottom":0},"overflowX":"visible","overflowY":"visible"}`
}

func recIn(screen string, vp int, src string) RecordInput {
	return RecordInput{Screen: screen, Viewport: vp, ScreenshotSrc: src,
		MeasurementJSON: measureJSON(strconv.Itoa(vp), "90")}
}

func readArtifactT(t *testing.T, repo, fp string) Artifact {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repo, ".znf", "ui-verify", fp+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var art Artifact
	if err := json.Unmarshal(b, &art); err != nil {
		t.Fatal(err)
	}
	return art
}

func TestRecord_Success(t *testing.T) {
	repo := t.TempDir()
	src := writeTempScreenshot(t, t.TempDir(), "shot.png", 128)
	d := stubDeps("abc1234567")

	s, err := Record(d, repo, recIn("home", 1920, src))
	if err != nil {
		t.Fatal(err)
	}
	if s.Verdict != "pass" || s.Viewport != 1920 {
		t.Fatalf("screen = %+v", s)
	}

	destPath := filepath.Join(repo, ".znf", "ui-verify", "abc1234567-home-1920.png")
	if info, err := os.Stat(destPath); err != nil || info.Size() != 128 {
		t.Fatalf("screenshot not copied to %s: %v", destPath, err)
	}
	giPath := filepath.Join(repo, ".znf", "ui-verify", ".gitignore")
	if b, err := os.ReadFile(giPath); err != nil || string(b) != "*\n" {
		t.Fatalf(".gitignore = %q, err=%v", b, err)
	}
	art := readArtifactT(t, repo, "abc1234567")
	if len(art.Screens) != 1 || art.Screens[0].Screen != "home" {
		t.Fatalf("artifact = %+v", art)
	}
	if art.Screens[0].Screenshot != "abc1234567-home-1920.png" {
		t.Errorf("screenshot field = %q", art.Screens[0].Screenshot)
	}
}

func TestRecord_ComputesVerdict(t *testing.T) {
	cases := []struct {
		name       string
		childRight string
		reason     string
		verdict    string
		overflows  []Overflow
	}{
		{name: "no overflow", childRight: "90", verdict: "pass"},
		{name: "right overflow 5", childRight: "101", verdict: "fail", overflows: []Overflow{{"right", 5}}},
		{name: "fail reason only", childRight: "90", reason: "label bị cắt", verdict: "fail"}, //znf:allow-lang
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			src := writeTempScreenshot(t, t.TempDir(), "s.png", 8)
			in := recIn("home", 1920, src)
			in.MeasurementJSON = measureJSON("1920", tc.childRight)
			in.FailReason = tc.reason
			s, err := Record(stubDeps("abc1234567"), repo, in)
			if err != nil {
				t.Fatal(err)
			}
			if s.Verdict != tc.verdict || s.FailReason != tc.reason || !reflect.DeepEqual(s.Overflows, tc.overflows) {
				t.Fatalf("got %+v", s)
			}
			stored := readArtifactT(t, repo, "abc1234567").Screens[0]
			if stored.Verdict != tc.verdict || stored.FailReason != tc.reason {
				t.Fatalf("stored %+v", stored)
			}
		})
	}
}

func TestRecord_ViewportMismatch(t *testing.T) {
	cases := []struct {
		name, innerWidth string
		viewport         int
		ok               bool
	}{
		{"mismatch", "1920", 1366, false},
		{"float form equal", "1366.0", 1366, true},
		{"below range", "100", 100, false},
		{"above range", "4000", 4000, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			src := writeTempScreenshot(t, t.TempDir(), "s.png", 8)
			in := recIn("home", tc.viewport, src)
			in.MeasurementJSON = measureJSON(tc.innerWidth, "90")
			_, err := Record(stubDeps("abc1234567"), repo, in)
			_, statErr := os.Stat(filepath.Join(repo, ".znf", "ui-verify", "abc1234567.json"))
			if tc.ok {
				if err != nil || statErr != nil {
					t.Fatalf("err=%v statErr=%v", err, statErr)
				}
				return
			}
			if exitcode.Code(err) != exitcode.BadArgs {
				t.Fatalf("code = %d, err = %v", exitcode.Code(err), err)
			}
			if !os.IsNotExist(statErr) {
				t.Fatalf("artifact must not be written, statErr=%v", statErr)
			}
		})
	}
}

func TestRecord_BadMeasurementWritesNothing(t *testing.T) {
	repo := t.TempDir()
	src := writeTempScreenshot(t, t.TempDir(), "s.png", 8)
	in := recIn("home", 1920, src)
	in.MeasurementJSON = `{"innerWidth":1920}`
	_, err := Record(stubDeps("abc1234567"), repo, in)
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("code = %d, err = %v", exitcode.Code(err), err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".znf", "ui-verify", "abc1234567.json")); !os.IsNotExist(err) {
		t.Fatalf("artifact json must not exist, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".znf", "ui-verify")); !os.IsNotExist(err) {
		t.Fatalf("ui-verify dir must not exist, err=%v", err)
	}
}

func TestRecord_Reject0Byte(t *testing.T) {
	repo := t.TempDir()
	src := writeTempScreenshot(t, t.TempDir(), "empty.png", 0)
	_, err := Record(stubDeps("abc1234567"), repo, recIn("home", 1920, src))
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("code = %d, want BadArgs (err=%v)", exitcode.Code(err), err)
	}
	artPath := filepath.Join(repo, ".znf", "ui-verify", "abc1234567.json")
	if _, err := os.Stat(artPath); !os.IsNotExist(err) {
		t.Errorf("artifact json should not be created, err=%v", err)
	}
}

func TestRecord_RejectMissingSrc(t *testing.T) {
	_, err := Record(stubDeps("abc1234567"), t.TempDir(),
		recIn("home", 1920, filepath.Join(t.TempDir(), "nope.png")))
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("code = %d, want BadArgs", exitcode.Code(err))
	}
}

func TestRecord_UpsertByScreenAndViewport(t *testing.T) {
	repo := t.TempDir()
	srcDir := t.TempDir()
	d := stubDeps("abc1234567")
	for _, step := range []struct {
		vp   int
		size int
	}{{1920, 10}, {1366, 20}, {1920, 30}} {
		src := writeTempScreenshot(t, srcDir, "s.png", step.size)
		if _, err := Record(d, repo, recIn("home", step.vp, src)); err != nil {
			t.Fatal(err)
		}
	}
	art := readArtifactT(t, repo, "abc1234567")
	if len(art.Screens) != 2 {
		t.Fatalf("want 2 entries, got %+v", art.Screens)
	}
	if art.Screens[0].Screenshot == art.Screens[1].Screenshot {
		t.Fatalf("viewports share a screenshot file: %+v", art.Screens)
	}
	// The re-recorded 1920 replaced the file in place.
	info, err := os.Stat(filepath.Join(repo, ".znf", "ui-verify", art.Screens[0].Screenshot))
	if err != nil || art.Screens[0].Viewport != 1920 || info.Size() != 30 {
		t.Fatalf("1920 entry = %+v size=%v err=%v", art.Screens[0], info, err)
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
		"-c core.quotePath=false diff --name-only origin/staging...HEAD": "main.go\nREADME.md",
		"-c core.quotePath=false status --porcelain -uall":              "",
	}, "fp000")
	state, _, err := Check(d, repo, "origin/staging")
	if state != "not_required" || err != nil {
		t.Errorf("state=%q err=%v", state, err)
	}
}

func TestCheck_SC2_Waived(t *testing.T) {
	repo := t.TempDir()
	d := checkDeps(t, map[string]string{
		"-c core.quotePath=false diff --name-only origin/staging...HEAD": "src/App.tsx",
		"-c core.quotePath=false status --porcelain -uall":              "",
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
		"-c core.quotePath=false diff --name-only origin/staging...HEAD": "src/App.tsx",
		"-c core.quotePath=false status --porcelain -uall":              "",
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
		"-c core.quotePath=false diff --name-only origin/staging...HEAD": "src/App.tsx",
		"-c core.quotePath=false status --porcelain -uall":              "",
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
		"-c core.quotePath=false diff --name-only origin/staging...HEAD": "src/App.tsx",
		"-c core.quotePath=false status --porcelain -uall":              "",
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
		"-c core.quotePath=false diff --name-only origin/staging...HEAD": "src/App.tsx",
		"-c core.quotePath=false status --porcelain -uall":              "",
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

// A route-shaped screen name ("chat/room") must not become a path segment of
// the screenshot file: the copy failed with "no such file or directory".
func TestRecord_ScreenNameWithSlash(t *testing.T) {
	repo := t.TempDir()
	src := writeTempScreenshot(t, t.TempDir(), "shot.png", 16)
	d := stubDeps("abc1234567")

	if _, err := Record(d, repo, recIn("chat/room", 1920, src)); err != nil {
		t.Fatal(err)
	}
	if _, err := Record(d, repo, recIn("chat-room", 1920, src)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(repo, ".znf", "ui-verify", "abc1234567.json"))
	if err != nil {
		t.Fatal(err)
	}
	var art Artifact
	if err := json.Unmarshal(b, &art); err != nil {
		t.Fatal(err)
	}
	if len(art.Screens) != 2 || art.Screens[0].Screen != "chat/room" || art.Screens[0].Screenshot == art.Screens[1].Screenshot {
		t.Fatalf("screens must be distinct files: %+v", art.Screens)
	}
	for _, sc := range art.Screens {
		if strings.Contains(sc.Screenshot, "/") {
			t.Fatalf("screenshot name has a path separator: %q", sc.Screenshot)
		}
		if _, err := os.Stat(filepath.Join(repo, ".znf", "ui-verify", sc.Screenshot)); err != nil {
			t.Fatalf("screenshot not written flat: %v", err)
		}
	}
}

func renderSetWithConfig(t *testing.T, cfg string, paths []string) map[string]bool {
	t.Helper()
	repo := t.TempDir()
	if cfg != "" {
		if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, ".claude", "worktree.json"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit := fakeRunGit(t, map[string]string{
		"-c core.quotePath=false diff --name-only origin/staging...HEAD": strings.Join(paths, "\n"),
		"-c core.quotePath=false status --porcelain -uall":              "",
	})
	got, err := RenderSet(Deps{RunGit: runGit}, repo, "origin/staging")
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, p := range got {
		set[p] = true
	}
	return set
}

func TestRenderSet_RenderPaths(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"src/constants/menu.js", true},
		{"src/components/Button.test.jsx", false},
		{"lib/a.js", true},
		{"lib", true},
		{"libx/a.js", false},
		{"README.md", false},
		{"docs/x.tsx", false},
	}
	var paths []string
	for _, c := range cases {
		paths = append(paths, c.path)
	}
	got := renderSetWithConfig(t, `{"uiVerify":{"renderPaths":["src/","lib"]}}`, paths)
	for _, c := range cases {
		if got[c.path] != c.want {
			t.Errorf("path %q: got %v, want %v", c.path, got[c.path], c.want)
		}
	}
}

func TestRenderSet_NoConfigKeepsRegex(t *testing.T) {
	got := renderSetWithConfig(t, "", []string{"src/constants/menu.js", "src/App.tsx"})
	if got["src/constants/menu.js"] || !got["src/App.tsx"] {
		t.Errorf("legacy regex not kept: %v", got)
	}
}
