package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
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
		t.Fatalf("stdout không phải JSON: %v (%q)", err, out.String())
	}
	if res.Kept != 1 || res.Refuted != 1 {
		t.Errorf("kept=%d refuted=%d, want 1/1", res.Kept, res.Refuted)
	}
}

func TestReviewVerify_FailOpenBadJSON(t *testing.T) {
	in := "khong-phai-json"
	var out, errb bytes.Buffer
	if err := runReviewVerify(strings.NewReader(in), &out, &errb, os.ReadFile); err != nil {
		t.Fatalf("fail-open không được trả err: %v", err)
	}
	if !strings.Contains(out.String(), "khong-phai-json") {
		t.Errorf("fail-open phải echo nguyên input, được: %q", out.String())
	}
}

func TestReviewVerify_EmptyStdin(t *testing.T) {
	var out, errb bytes.Buffer
	if err := runReviewVerify(strings.NewReader("   \n"), &out, &errb, os.ReadFile); err != nil {
		t.Fatalf("empty stdin: %v", err)
	}
	if !strings.Contains(out.String(), `"kept":0`) {
		t.Errorf("empty stdin phải ra kept 0, được: %q", out.String())
	}
}

func TestRunReviewBundle_Passthrough(t *testing.T) {
	// numstat tổng 300 <= 2000 → passthrough.
	rundiff := func(string) ([]byte, error) {
		return []byte("100\t50\ta/x.go\n100\t50\ta/y.go\n"), nil
	}
	var out, errb bytes.Buffer
	if err := runReviewBundle("HEAD", rundiff, &out, &errb); err != nil {
		t.Fatalf("err=%v, want nil (fail-open)", err)
	}
	var p review.Plan
	if err := json.Unmarshal(out.Bytes(), &p); err != nil {
		t.Fatalf("stdout không phải JSON Plan: %v (%q)", err, out.String())
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
		t.Fatalf("stdout không phải JSON: %v", err)
	}
	if p.Verdict != "bundle" || len(p.Bundles) == 0 {
		t.Errorf("verdict=%q bundles=%d, want bundle với >=1 bundle", p.Verdict, len(p.Bundles))
	}
}

func TestRunReviewBundle_FailOpenOnDiffError(t *testing.T) {
	// git diff lỗi (vd ngoài git repo) → passthrough, KHÔNG trả error.
	rundiff := func(string) ([]byte, error) { return nil, errors.New("not a git repo") }
	var out, errb bytes.Buffer
	if err := runReviewBundle("HEAD", rundiff, &out, &errb); err != nil {
		t.Fatalf("err=%v, want nil (fail-open)", err)
	}
	var p review.Plan
	if err := json.Unmarshal(out.Bytes(), &p); err != nil {
		t.Fatalf("stdout không phải JSON: %v (%q)", err, out.String())
	}
	if p.Verdict != "passthrough" {
		t.Errorf("verdict=%q, want passthrough khi diff lỗi", p.Verdict)
	}
}

func TestReviewBundle_BinaryLineParsedAsZero(t *testing.T) {
	// dòng binary "-\t-\tpath" → LOC 0; chỉ file text đẩy tổng.
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
		t.Errorf("total=%d, want 2100 (binary đếm 0)", p.TotalLOC)
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
		t.Errorf("review-verify chưa đăng ký trong root")
	}
}
