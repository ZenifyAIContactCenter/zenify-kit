package uiverify

import (
	"encoding/json"
	"errors"
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

const (
	waiverCommittedKey = "-c core.quotePath=false diff origin/staging...HEAD -- src/App.tsx src/b.css"
	waiverWorkingKey   = "-c core.quotePath=false diff HEAD -- src/App.tsx src/b.css"
)

var waiverFiles = []string{"src/App.tsx", "src/b.css"}

func TestWaiver(t *testing.T) {
	cases := []struct {
		name       string
		committed  string
		working    string
		wantOK     bool
		wantReason string
	}{
		{"added line waives", "+// znf:ui-verify-ok: copy only\n", "", true, "copy only"},
		{"removed line does not", "-// znf:ui-verify-ok: legacy\n", "", false, ""},
		{"empty reason does not", "+// znf:ui-verify-ok:\n", "", false, ""},
		{"blank reason does not", "+// znf:ui-verify-ok:   \n", "", false, ""},
		{"empty reason then valid later line", "+// znf:ui-verify-ok:\n+// znf:ui-verify-ok: later\n", "", true, "later"},
		{"context line does not", " context znf:ui-verify-ok: x\n", "", false, ""},
		{"working tree added line waives", "no marker\n", "+// znf:ui-verify-ok: wip\n", true, "wip"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			runGit := fakeRunGit(t, map[string]string{waiverCommittedKey: c.committed, waiverWorkingKey: c.working})
			reason, ok := Waiver(Deps{RunGit: runGit}, "/repo", "origin/staging", waiverFiles)
			if ok != c.wantOK || reason != c.wantReason {
				t.Errorf("got (%q, %v), want (%q, %v)", reason, ok, c.wantReason, c.wantOK)
			}
		})
	}
}

func TestWaiver_IgnoresPlusPlusHeader(t *testing.T) {
	runGit := fakeRunGit(t, map[string]string{
		waiverCommittedKey: "+++ b/znf:ui-verify-ok: x.tsx\n",
		waiverWorkingKey:   "",
	})
	if _, ok := Waiver(Deps{RunGit: runGit}, "/repo", "origin/staging", waiverFiles); ok {
		t.Fatal("a +++ file header must not waive")
	}
}

// fakeRunGit fails the test on any key other than the exact pathspec, so a
// marker in docs/notes.md can never be seen: it is not in files.
func TestWaiver_OnlyRenderFiles(t *testing.T) {
	runGit := fakeRunGit(t, map[string]string{
		waiverCommittedKey: "no marker\n",
		waiverWorkingKey:   "",
	})
	if _, ok := Waiver(Deps{RunGit: runGit}, "/repo", "origin/staging", waiverFiles); ok {
		t.Fatal("expected no waiver")
	}
}

func TestWaiver_GitErrorIsNoWaiver(t *testing.T) {
	runGit := func(_ string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "diff HEAD") {
			return "+// znf:ui-verify-ok: wip\n", nil
		}
		return "", errors.New("boom")
	}
	if reason, ok := Waiver(Deps{RunGit: runGit}, "/repo", "origin/staging", waiverFiles); ok || reason != "" {
		t.Fatalf("got (%q, %v), want no waiver on git error", reason, ok)
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
		{name: "fail reason only", childRight: "90", reason: "label clipped", verdict: "fail"}, //znf:allow-lang
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
		Screens: []Screen{{Screen: "home", Viewport: 1440, Screenshot: "shot.png", Verdict: "pass"}},
	})
	n, problems := Validate(repo, "fp1", nil)
	if len(problems) != 0 || n != 1 {
		t.Errorf("n=%d problems=%v, want 1 none", n, problems)
	}
}

func TestValidate_MissingScreenshot(t *testing.T) {
	repo := t.TempDir()
	writeArtifact(t, repo, "fp1", Artifact{
		FP:      "fp1",
		Screens: []Screen{{Screen: "home", Viewport: 1440, Screenshot: "missing.png", Verdict: "pass"}},
	})
	_, problems := Validate(repo, "fp1", nil)
	if len(problems) != 1 || problems[0] != "home@1440: screenshot missing" {
		t.Errorf("problems=%v", problems)
	}
}

func TestValidate_MissingJSON(t *testing.T) {
	repo := t.TempDir()
	n, problems := Validate(repo, "nope", nil)
	if n != 0 || len(problems) != 1 || problems[0] != "no artifact for fp=nope" {
		t.Errorf("n=%d problems=%v", n, problems)
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
		"-c core.quotePath=false diff --name-only origin/staging...HEAD":    "src/App.tsx",
		"-c core.quotePath=false status --porcelain -uall":                  "",
		"-c core.quotePath=false diff origin/staging...HEAD -- src/App.tsx": "+// znf:ui-verify-ok: reviewed manually",
		"-c core.quotePath=false diff HEAD -- src/App.tsx":                  "",
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
	writeArtifact(t, repo, fp, Artifact{FP: fp, Screens: []Screen{{Screen: "home", Viewport: 1440, Screenshot: "shot.png", Verdict: "pass"}}})

	d := checkDeps(t, map[string]string{
		"-c core.quotePath=false diff --name-only origin/staging...HEAD":    "src/App.tsx",
		"-c core.quotePath=false status --porcelain -uall":                  "",
		"-c core.quotePath=false diff origin/staging...HEAD -- src/App.tsx": "",
		"-c core.quotePath=false diff HEAD -- src/App.tsx":                  "",
	}, fp)
	state, _, err := Check(d, repo, "origin/staging")
	if state != "verified" || err != nil {
		t.Errorf("state=%q err=%v", state, err)
	}
}

func TestCheck_SC4_RequiredMissingArtifact(t *testing.T) {
	repo := t.TempDir()
	d := checkDeps(t, map[string]string{
		"-c core.quotePath=false diff --name-only origin/staging...HEAD":    "src/App.tsx",
		"-c core.quotePath=false status --porcelain -uall":                  "",
		"-c core.quotePath=false diff origin/staging...HEAD -- src/App.tsx": "",
		"-c core.quotePath=false diff HEAD -- src/App.tsx":                  "",
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
	writeArtifact(t, repo, fp, Artifact{FP: fp, Screens: []Screen{{Screen: "home", Viewport: 1440, Screenshot: "missing.png", Verdict: "pass"}}})

	d := checkDeps(t, map[string]string{
		"-c core.quotePath=false diff --name-only origin/staging...HEAD":    "src/App.tsx",
		"-c core.quotePath=false status --porcelain -uall":                  "",
		"-c core.quotePath=false diff origin/staging...HEAD -- src/App.tsx": "",
		"-c core.quotePath=false diff HEAD -- src/App.tsx":                  "",
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
	writeArtifact(t, repo, oldFP, Artifact{FP: oldFP, Screens: []Screen{{Screen: "home", Viewport: 1440, Screenshot: "shot.png", Verdict: "pass"}}})

	newFP := "fpnew000002"
	d := checkDeps(t, map[string]string{
		"-c core.quotePath=false diff --name-only origin/staging...HEAD":    "src/App.tsx",
		"-c core.quotePath=false status --porcelain -uall":                  "",
		"-c core.quotePath=false diff origin/staging...HEAD -- src/App.tsx": "",
		"-c core.quotePath=false diff HEAD -- src/App.tsx":                  "",
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

// gateRepo writes worktree.json (cfg, "" = none) and a shot.png, and returns
// the repo plus Deps whose fingerprint is fp and whose diff has one .tsx file.
func gateRepo(t *testing.T, cfg, fp string) (string, Deps) {
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
	dir := filepath.Join(repo, ".znf", "ui-verify")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shot.png"), []byte{1}, 0o600); err != nil {
		t.Fatal(err)
	}
	return repo, checkDeps(t, map[string]string{
		"-c core.quotePath=false diff --name-only origin/staging...HEAD": "src/App.tsx",
	}, fp)
}

func shotScreen(name string, vp int, verdict string) Screen {
	return Screen{Screen: name, Viewport: vp, Screenshot: "shot.png", Verdict: verdict}
}

const threeViewports = `{"uiVerify":{"viewports":[1920,1366,1280]}}`

func TestCheck_MissingViewport(t *testing.T) {
	repo, d := gateRepo(t, threeViewports, "fpmissvp01")
	writeArtifact(t, repo, "fpmissvp01", Artifact{FP: "fpmissvp01", Screens: []Screen{
		shotScreen("ticket-detail", 1920, "pass"), shotScreen("ticket-detail", 1366, "pass"),
	}})
	state, msg, err := Check(d, repo, "origin/staging")
	if state != "required" || exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("state=%q err=%v", state, err)
	}
	if !strings.Contains(msg, "ticket-detail: missing viewport 1280") {
		t.Errorf("msg=%q", msg)
	}
	if !strings.HasPrefix(msg, "ui-verify required — ") || !strings.HasSuffix(msg, "(rendering files: src/App.tsx)") {
		t.Errorf("msg shape=%q", msg)
	}
}

func TestCheck_AllViewportsPass(t *testing.T) {
	repo, d := gateRepo(t, threeViewports, "fpallvp001")
	var screens []Screen
	for _, name := range []string{"home", "ticket-detail"} {
		for _, vp := range []int{1920, 1366, 1280} {
			screens = append(screens, shotScreen(name, vp, "pass"))
		}
	}
	writeArtifact(t, repo, "fpallvp001", Artifact{FP: "fpallvp001", Screens: screens})
	state, msg, err := Check(d, repo, "origin/staging")
	if state != "verified" || err != nil {
		t.Errorf("state=%q msg=%q err=%v", state, msg, err)
	}
}

func TestCheck_FailEntryNoConfig(t *testing.T) {
	repo, d := gateRepo(t, "", "fpfailnocf")
	writeArtifact(t, repo, "fpfailnocf", Artifact{FP: "fpfailnocf", Screens: []Screen{
		{Screen: "home", Viewport: 1366, Screenshot: "shot.png", Verdict: "fail",
			Overflows: []Overflow{{Edge: "right", Px: 5}}},
	}})
	state, msg, err := Check(d, repo, "origin/staging")
	if state != "required" || exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("state=%q err=%v", state, err)
	}
	if !strings.Contains(msg, "home@1366: fail (right 5px)") {
		t.Errorf("msg=%q", msg)
	}
}

func TestCheck_FailReasonOnly(t *testing.T) {
	repo, d := gateRepo(t, "", "fpfailrsn1")
	writeArtifact(t, repo, "fpfailrsn1", Artifact{FP: "fpfailrsn1", Screens: []Screen{
		{Screen: "home", Viewport: 1920, Screenshot: "shot.png", Verdict: "fail", FailReason: "label clipped"},
	}})
	_, msg, _ := Check(d, repo, "origin/staging")
	if !strings.Contains(msg, "home@1920: fail (label clipped)") {
		t.Errorf("msg=%q", msg)
	}
}

func TestCheck_NoConfigAnyViewportPasses(t *testing.T) {
	repo, d := gateRepo(t, "", "fpnocfgok1")
	writeArtifact(t, repo, "fpnocfgok1", Artifact{FP: "fpnocfgok1", Screens: []Screen{shotScreen("home", 1440, "pass")}})
	state, _, err := Check(d, repo, "origin/staging")
	if state != "verified" || err != nil {
		t.Errorf("state=%q err=%v", state, err)
	}
}

func TestCheck_InvalidConfigIsBadArgs(t *testing.T) {
	repo, d := gateRepo(t, `{"uiVerify":{"viewports":[100]}}`, "fpbadcfg01")
	_, _, err := Check(d, repo, "origin/staging")
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("code=%d, want BadArgs", exitcode.Code(err))
	}
}

func TestValidate_InvalidEntries(t *testing.T) {
	repo, _ := gateRepo(t, "", "x")
	writeArtifact(t, repo, "fpinv00001", Artifact{FP: "fpinv00001", Screens: []Screen{
		{Screen: "a", Viewport: 0, Screenshot: "shot.png", Verdict: "pass"},
		{Screen: "b", Viewport: 1440, Screenshot: "shot.png", Verdict: ""},
	}})
	_, problems := Validate(repo, "fpinv00001", nil)
	if len(problems) != 2 || problems[0] != "a: invalid entry" || problems[1] != "b: invalid entry" {
		t.Errorf("problems=%v", problems)
	}
}

func TestValidate_LegacyArtifactIsInvalid(t *testing.T) {
	repo, _ := gateRepo(t, "", "x")
	legacy := `{"fp":"fpleg00001","recorded_at":"x","screens":[{"screen":"home","screenshot":"shot.png","measurement":{"child_right":10}}]}`
	if err := os.WriteFile(filepath.Join(repo, ".znf", "ui-verify", "fpleg00001.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, problems := Validate(repo, "fpleg00001", []int{1920}); len(problems) == 0 {
		t.Error("legacy artifact must be invalid")
	}
	if err := os.WriteFile(filepath.Join(repo, ".znf", "ui-verify", "fpbad00001.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, problems := Validate(repo, "fpbad00001", nil); len(problems) == 0 {
		t.Error("unparsable artifact must be invalid")
	}
}
