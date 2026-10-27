package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/review"
)

func TestReviewVerify_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/a.go", []byte("1\nfoo()\n3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	in := `[{"file":"` + dir + `/a.go","line":"2","evidence":"foo()"},` +
		`{"file":"` + dir + `/a.go","line":"2","evidence":"khong-co"}]`
	var out, errb bytes.Buffer
	if err := runReviewVerify(strings.NewReader(in), &out, &errb, os.ReadFile); err != nil {
		t.Fatalf("runReviewVerify: %v", err)
	}
	var res struct {
		Kept, Refuted int
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("stdout is not JSON: %v (%q)", err, out.String())
	}
	if res.Kept != 1 || res.Refuted != 1 {
		t.Errorf("kept=%d refuted=%d, want 1/1", res.Kept, res.Refuted)
	}
}

func TestReviewVerify_FailOpenBadJSON(t *testing.T) {
	in := "khong-phai-json"
	var out, errb bytes.Buffer
	if err := runReviewVerify(strings.NewReader(in), &out, &errb, os.ReadFile); err != nil {
		t.Fatalf("fail-open must not return err: %v", err)
	}
	if !strings.Contains(out.String(), "khong-phai-json") {
		t.Errorf("fail-open must echo the input verbatim, got: %q", out.String())
	}
}

func TestReviewVerify_EmptyStdin(t *testing.T) {
	var out, errb bytes.Buffer
	if err := runReviewVerify(strings.NewReader("   \n"), &out, &errb, os.ReadFile); err != nil {
		t.Fatalf("empty stdin: %v", err)
	}
	if !strings.Contains(out.String(), `"kept":0`) {
		t.Errorf("empty stdin must yield kept 0, got: %q", out.String())
	}
}

func TestRunReviewBundle_Passthrough(t *testing.T) {
	// numstat total 300 <= 2000 → passthrough.
	rundiff := func(string) ([]byte, error) {
		return []byte("100\t50\ta/x.go\n100\t50\ta/y.go\n"), nil
	}
	var out, errb bytes.Buffer
	if err := runReviewBundle("HEAD", rundiff, &out, &errb); err != nil {
		t.Fatalf("err=%v, want nil (fail-open)", err)
	}
	var p review.Plan
	if err := json.Unmarshal(out.Bytes(), &p); err != nil {
		t.Fatalf("stdout is not JSON Plan: %v (%q)", err, out.String())
	}
	if p.Verdict != "passthrough" {
		t.Errorf("verdict=%q, want passthrough", p.Verdict)
	}
}

func TestRunReviewBundle_Bundle(t *testing.T) {
	// 6 file 250/250 = 3000 > 2000 → bundle.
	var lines string
	for i := 0; i < 6; i++ {
		lines += "250\t250\tpkg/f" + string(rune('a'+i)) + ".go\n"
	}
	rundiff := func(string) ([]byte, error) { return []byte(lines), nil }
	var out, errb bytes.Buffer
	if err := runReviewBundle("HEAD", rundiff, &out, &errb); err != nil {
		t.Fatalf("err=%v, want nil", err)
	}
	var p review.Plan
	if err := json.Unmarshal(out.Bytes(), &p); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if p.Verdict != "bundle" || len(p.Bundles) == 0 {
		t.Errorf("verdict=%q bundles=%d, want bundle with >=1 bundle", p.Verdict, len(p.Bundles))
	}
}

func TestRunReviewBundle_FailOpenOnDiffError(t *testing.T) {
	// git diff error (e.g. outside a git repo) → passthrough, does NOT return an error.
	rundiff := func(string) ([]byte, error) { return nil, errors.New("not a git repo") }
	var out, errb bytes.Buffer
	if err := runReviewBundle("HEAD", rundiff, &out, &errb); err != nil {
		t.Fatalf("err=%v, want nil (fail-open)", err)
	}
	var p review.Plan
	if err := json.Unmarshal(out.Bytes(), &p); err != nil {
		t.Fatalf("stdout is not JSON: %v (%q)", err, out.String())
	}
	if p.Verdict != "passthrough" {
		t.Errorf("verdict=%q, want passthrough when diff errors", p.Verdict)
	}
}

func TestReviewBundle_BinaryLineParsedAsZero(t *testing.T) {
	// a binary line "-\t-\tpath" → LOC 0; only text files push up the total.
	rundiff := func(string) ([]byte, error) {
		return []byte("-\t-\tassets/logo.png\n1500\t600\ta/big.go\n"), nil
	}
	var out, errb bytes.Buffer
	if err := runReviewBundle("HEAD", rundiff, &out, &errb); err != nil {
		t.Fatalf("err=%v", err)
	}
	var p review.Plan
	_ = json.Unmarshal(out.Bytes(), &p)
	if p.TotalLOC != 2100 {
		t.Errorf("total=%d, want 2100 (binary counts 0)", p.TotalLOC)
	}
}

func TestReviewVerify_Registered(t *testing.T) {
	root := NewRootCmd()
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "review-verify" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("review-verify not registered in root")
	}
}

func TestRemovedReviewSubcommands(t *testing.T) {
	root := NewRootCmd()
	for _, name := range []string{"review-doctrine", "review-advise-gate", "review-log"} {
		if c, _, err := root.Find([]string{name}); err == nil && c != root {
			t.Errorf("subcommand %q still registered", name)
		}
	}
	if c, _, err := root.Find([]string{"review-verify"}); err != nil || c == root {
		t.Error("review-verify must stay registered")
	}
}

func TestReviewVerify_KeepsIntentDimension(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	if err := os.WriteFile(p, []byte("line one\nthe quoted line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	in := `[{"dimension":"intent","severity":"HIGH","title":"t","file":"` + p + `","line":"2","issue":"i","fix":"f","evidence":"the quoted line"}]`
	var out bytes.Buffer
	if err := runReviewVerify(strings.NewReader(in), &out, io.Discard, os.ReadFile); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"kept":1`) || !strings.Contains(out.String(), `"dimension":"intent"`) {
		t.Errorf("intent finding not kept: %s", out.String())
	}
}

// znf:nonstop reviews an uncommitted tree: untracked new files must reach the bundler as numstat
// lines, or a large diff bundles without them and they are never reviewed.
func TestUntrackedNumstat_ListsNewFiles(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "new.go"), []byte("a\nb\nc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := untrackedNumstat(dir)
	if err != nil {
		t.Fatalf("untrackedNumstat: %v", err)
	}
	if string(got) != "3\t0\tpkg/new.go\n" {
		t.Errorf("got %q, want %q", got, "3\t0\tpkg/new.go\n")
	}
}

// A dangling symlink among the untracked files is listed with 0 lines instead of failing the
// whole bundle plan.
func TestUntrackedNumstat_SymlinkNotRead(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.Symlink("missing-target", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	got, err := untrackedNumstat(dir)
	if err != nil {
		t.Fatalf("untrackedNumstat: %v", err)
	}
	if string(got) != "0\t0\tlink\n" {
		t.Errorf("got %q, want %q", got, "0\t0\tlink\n")
	}
}

// An untracked binary file counts 0 lines, like git's "-\t-" numstat, so new assets cannot push
// a plan to too-large and stop the review.
func TestUntrackedNumstat_BinaryCountsZero(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	data := append([]byte{0x89, 'P', 'N', 'G', 0x00}, bytes.Repeat([]byte("\n"), 700)...)
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := untrackedNumstat(dir)
	if err != nil {
		t.Fatalf("untrackedNumstat: %v", err)
	}
	if string(got) != "0\t0\tlogo.png\n" {
		t.Errorf("got %q, want %q", got, "0\t0\tlogo.png\n")
	}
}
