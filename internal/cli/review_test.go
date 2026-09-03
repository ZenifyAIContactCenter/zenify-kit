package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
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
