package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleRecord() Record {
	return Record{
		TS: "2026-09-04T01:22:33Z", Repo: "zenify-kit", Base: "5b1d695", Head: "6678c289abcd",
		Tier: "T2", Outcome: "reviewed",
		Findings: FindingCounts{High: 1, Medium: 2},
		Kept:     3, Refuted: 1, Shippable: true,
		Signals: []string{"shared-contract touched"}, Categories: []string{"contracts", "bugs", "bugs"},
	}
}

func TestWriteRecord_RoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "review-log")
	path, err := WriteRecord(dir, sampleRecord())
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.HasSuffix(path, ".json") {
		t.Errorf("path không .json: %s", path)
	}
	// 0600
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, muốn 600", fi.Mode().Perm())
	}
	// .gitignore self-ignore
	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf(".gitignore chưa tạo: %v", err)
	}
	if strings.TrimSpace(string(gi)) != "*" {
		t.Errorf(".gitignore = %q, muốn *", string(gi))
	}
	recs, err := LoadRecords(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Tier != "T2" || recs[0].Kept != 3 {
		t.Errorf("round-trip sai: %+v", recs)
	}
}

func TestWriteRecord_NilSlicesEncodeEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rl")
	r := sampleRecord()
	r.Signals = nil
	r.Categories = nil
	path, err := WriteRecord(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	s := string(b)
	if strings.Contains(s, "null") {
		t.Errorf("encode ra null: %s", s)
	}
	if !strings.Contains(s, `"signals":[]`) || !strings.Contains(s, `"categories":[]`) {
		t.Errorf("nil slice không thành []: %s", s)
	}
}

func TestWriteRecord_SanitizeHead(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rl")
	r := sampleRecord()
	r.Head = "../../etc/passwd"
	path, err := WriteRecord(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	// file phải nằm TRONG dir, không escape
	if filepath.Dir(path) != dir {
		t.Errorf("head độc hại escape dir: %s", path)
	}
}

func TestLoadRecords_MissingDir(t *testing.T) {
	recs, err := LoadRecords(filepath.Join(t.TempDir(), "khong-ton-tai"))
	if err != nil {
		t.Fatalf("dir thiếu phải nil err: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("muốn rỗng: %v", recs)
	}
}

func TestLoadRecords_SkipsCorrupt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rl")
	if _, err := WriteRecord(dir, sampleRecord()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	recs, err := LoadRecords(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Errorf("file hỏng phải bị skip, giữ 1 record: %d", len(recs))
	}
}

func TestSummarize_Math(t *testing.T) {
	recs := []Record{
		{Tier: "T2", Findings: FindingCounts{High: 1, Medium: 2}, Kept: 3, Refuted: 1, Shippable: true, Categories: []string{"contracts", "bugs", "bugs"}},
		{Tier: "T1", Findings: FindingCounts{Low: 1}, Kept: 1, Refuted: 0, Shippable: false, Categories: []string{"types"}},
		{Tier: "T2", Findings: FindingCounts{}, Kept: 0, Refuted: 2, Shippable: true, Categories: []string{}},
	}
	s := Summarize(recs)
	if s.Total != 3 {
		t.Errorf("Total=%d", s.Total)
	}
	if s.ByTier["T2"] != 2 || s.ByTier["T1"] != 1 {
		t.Errorf("ByTier=%v", s.ByTier)
	}
	if s.Findings.High != 1 || s.Findings.Medium != 2 || s.Findings.Low != 1 {
		t.Errorf("Findings=%+v", s.Findings)
	}
	if s.Kept != 4 || s.Refuted != 3 {
		t.Errorf("Kept=%d Refuted=%d", s.Kept, s.Refuted)
	}
	// refute rate = 3/(4+3)
	if s.RefuteRate < 0.428 || s.RefuteRate > 0.429 {
		t.Errorf("RefuteRate=%v", s.RefuteRate)
	}
	if s.ShippableN != 2 {
		t.Errorf("ShippableN=%d", s.ShippableN)
	}
	if len(s.TopCategory) == 0 || s.TopCategory[0].Name != "bugs" || s.TopCategory[0].N != 2 {
		t.Errorf("TopCategory[0] phải bugs/2: %+v", s.TopCategory)
	}
}

func TestSummarize_RefuteRateZeroDenom(t *testing.T) {
	s := Summarize([]Record{{Tier: "T1", Kept: 0, Refuted: 0}})
	if s.RefuteRate != 0 {
		t.Errorf("mẫu số 0 → RefuteRate 0, được %v", s.RefuteRate)
	}
}

var _ = json.Marshal // giữ import nếu chưa dùng trực tiếp
