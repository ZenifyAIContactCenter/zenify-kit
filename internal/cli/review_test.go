package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/review"
	"github.com/spf13/cobra"
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

func TestRunReviewDoctrine_StripsVerdict(t *testing.T) {
	var out, errb bytes.Buffer
	in := strings.NewReader("✅ all good\n12/12 pass — pm test src/x")
	if err := runReviewDoctrine(in, &out, &errb); err != nil {
		t.Fatalf("err=%v, want nil", err)
	}
	var r doctrineResult
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("bad json: %v; out=%s", err, out.String())
	}
	if r.Verified != "12/12 pass — pm test src/x" {
		t.Fatalf("verified=%q", r.Verified)
	}
	if len(r.Stripped) != 1 || r.Stripped[0] != "✅ all good" {
		t.Fatalf("stripped=%v", r.Stripped)
	}
}

func TestRunReviewDoctrine_FailOpenEmpty(t *testing.T) {
	var out, errb bytes.Buffer
	if err := runReviewDoctrine(strings.NewReader(""), &out, &errb); err != nil {
		t.Fatalf("err=%v, want nil", err)
	}
	var r doctrineResult
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if r.Verified != "" || len(r.Stripped) != 0 {
		t.Fatalf("want empty; got %q / %v", r.Verified, r.Stripped)
	}
}

func TestReviewDoctrineCmd_Hidden(t *testing.T) {
	c := newReviewDoctrineCmd()
	if !c.Hidden {
		t.Fatal("review-doctrine phải Hidden")
	}
	if c.Use != "review-doctrine" {
		t.Fatalf("Use=%q", c.Use)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestRunReviewAdviseGate_Advises(t *testing.T) {
	in := `{"shared":true,"added":10,"findings":[],"shippable":true}`
	var out, errb bytes.Buffer
	if err := runReviewAdviseGate(strings.NewReader(in), &out, &errb); err != nil {
		t.Fatal(err)
	}
	var res adviseResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("output không phải JSON: %v (%s)", err, out.String())
	}
	if !res.Advise {
		t.Error("shared phải advise")
	}
	if !contains(res.Signals, "shared-contract touched") {
		t.Errorf("thiếu signal: %v", res.Signals)
	}
}

func TestRunReviewAdviseGate_FailOpenEmpty(t *testing.T) {
	var out, errb bytes.Buffer
	if err := runReviewAdviseGate(strings.NewReader(""), &out, &errb); err != nil {
		t.Fatal(err)
	}
	var res adviseResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("empty phải emit JSON hợp lệ: %v", err)
	}
	if res.Advise {
		t.Error("empty → advise=false")
	}
	if res.Signals == nil {
		t.Error("signals phải [] không null")
	}
}

func TestRunReviewAdviseGate_FailOpenMalformed(t *testing.T) {
	var out, errb bytes.Buffer
	if err := runReviewAdviseGate(strings.NewReader("{not json"), &out, &errb); err != nil {
		t.Fatal(err)
	}
	var res adviseResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("malformed phải fail-open JSON: %v", err)
	}
	if res.Advise {
		t.Error("malformed → advise=false")
	}
}

func TestReviewAdviseGateCmd_Hidden(t *testing.T) {
	c := newReviewAdviseGateCmd()
	if !c.Hidden {
		t.Error("phải Hidden")
	}
	if c.Use != "review-advise-gate" {
		t.Errorf("Use = %q", c.Use)
	}
}

func TestReviewLogDir_UsesGitCommonDir(t *testing.T) {
	// inject fake git → gcd tuyệt đối "/x/y/.git" → dir = /x/y/.znf/review-log
	dir, err := reviewLogDir(func(args ...string) ([]byte, error) {
		return []byte("/x/y/.git\n"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join("/x/y", ".znf", "review-log") {
		t.Errorf("dir = %s", dir)
	}
}

func TestReviewLogDir_GitError(t *testing.T) {
	_, err := reviewLogDir(func(args ...string) ([]byte, error) {
		return nil, os.ErrNotExist
	})
	if err == nil {
		t.Error("git lỗi phải trả error để caller fail-open")
	}
}

func TestRunReviewLogRecord_WritesThenFailOpen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rl")
	dirFn := func() (string, error) { return dir, nil }
	rec := `{"ts":"2026-09-04T01:00:00Z","repo":"r","base":"a","head":"deadbeef","tier":"T2","outcome":"reviewed","findings":{"high":1},"kept":1,"refuted":0,"shippable":true,"signals":[],"categories":["bugs"]}`
	var errb bytes.Buffer
	if err := runReviewLogRecord(strings.NewReader(rec), &errb, dirFn); err != nil {
		t.Fatalf("record hợp lệ phải nil err: %v", err)
	}
	recs, _ := review.LoadRecords(dir)
	if len(recs) != 1 {
		t.Fatalf("chưa ghi record: %d", len(recs))
	}
	// empty stdin → không ghi thêm, vẫn nil
	if err := runReviewLogRecord(strings.NewReader(""), &errb, dirFn); err != nil {
		t.Fatalf("empty phải nil: %v", err)
	}
	// malformed → không ghi thêm, vẫn nil
	if err := runReviewLogRecord(strings.NewReader("{bad"), &errb, dirFn); err != nil {
		t.Fatalf("malformed phải nil: %v", err)
	}
	recs, _ = review.LoadRecords(dir)
	if len(recs) != 1 {
		t.Errorf("empty/malformed không được ghi thêm, còn %d", len(recs))
	}
}

func TestRunReviewLogRecord_DirErrorFailOpen(t *testing.T) {
	dirFn := func() (string, error) { return "", os.ErrNotExist }
	rec := `{"ts":"t","head":"h","tier":"T1"}`
	var errb bytes.Buffer
	if err := runReviewLogRecord(strings.NewReader(rec), &errb, dirFn); err != nil {
		t.Errorf("resolve-dir lỗi phải nil (fail-open): %v", err)
	}
}

func TestRunReviewLogShow_Empty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "empty")
	dirFn := func() (string, error) { return dir, nil }
	var out, errb bytes.Buffer
	if err := runReviewLogShow(&out, &errb, false, dirFn); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no reviews logged yet") {
		t.Errorf("empty phải báo no reviews: %q", out.String())
	}
}

func TestRunReviewLogShow_JSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rl")
	if _, err := review.WriteRecord(dir, review.Record{TS: "2026-09-04T01:00:00Z", Head: "abc12345", Tier: "T2", Kept: 1, Categories: []string{"bugs"}}); err != nil {
		t.Fatal(err)
	}
	dirFn := func() (string, error) { return dir, nil }
	var out, errb bytes.Buffer
	if err := runReviewLogShow(&out, &errb, true, dirFn); err != nil {
		t.Fatal(err)
	}
	var recs []review.Record
	if err := json.Unmarshal(out.Bytes(), &recs); err != nil {
		t.Fatalf("--json phải in mảng record: %v (%s)", err, out.String())
	}
	if len(recs) != 1 || recs[0].Tier != "T2" {
		t.Errorf("json sai: %+v", recs)
	}
}

func TestReviewLogCmd_RecordChildHidden(t *testing.T) {
	c := newReviewLogCmd()
	if c.Use != "review-log" {
		t.Errorf("parent Use = %q", c.Use)
	}
	var rec *cobra.Command
	for _, sub := range c.Commands() {
		if sub.Use == "record" {
			rec = sub
		}
	}
	if rec == nil {
		t.Fatal("thiếu child record")
	}
	if !rec.Hidden {
		t.Error("record phải Hidden")
	}
}
