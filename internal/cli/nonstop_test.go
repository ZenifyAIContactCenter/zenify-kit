package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func execNonstopStats(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"nonstop", "stats"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestNonstopStats_JSON(t *testing.T) {
	dir := t.TempDir()
	rows := "2026-10-28 · a · hub · fix · S · S · ~2m · 1 · 0 · -\n" +
		"2026-10-28 · b · hub · fix · L · M · ~10m · 2 · 0 · -\n" +
		"junk\n"
	if err := os.WriteFile(filepath.Join(dir, "estimates.md"), []byte(rows), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := execNonstopStats(t, "--json", "--dir", dir, "--today", "2026-10-28")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Missing bool `json:"missing"`
		Scale   struct {
			Source string `json:"source"`
			S      int    `json:"S"`
		} `json:"scale"`
		Today struct {
			Hit, Total, Over, Points int
			IDs                      []string `json:"ids"`
		} `json:"today"`
		Malformed int `json:"malformed"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if got.Missing || got.Scale.Source != "default" || got.Scale.S != 5 {
		t.Errorf("scale/missing: %+v", got)
	}
	if got.Today.Hit != 1 || got.Today.Total != 2 || got.Today.Over != 1 || got.Today.Points != 3 || len(got.Today.IDs) != 2 || got.Malformed != 1 {
		t.Errorf("today: %+v", got)
	}
}

func TestNonstopStats_MissingFile(t *testing.T) {
	out, err := execNonstopStats(t, "--json", "--dir", t.TempDir(), "--today", "2026-10-28")
	if err != nil {
		t.Fatalf("missing file must exit 0: %v", err)
	}
	if !strings.Contains(out, `"missing": true`) {
		t.Errorf("want missing:true, got %s", out)
	}
}

func TestNonstopStats_UnreadablePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "estimates.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := execNonstopStats(t, "--json", "--dir", dir); err == nil {
		t.Fatal("a directory named estimates.md must fail")
	}
}

func TestNonstopStats_HumanTable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "estimates.md"), []byte("2026-10-28 · a · hub · fix · S · S · ~2m · 1 · 0 · -\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := execNonstopStats(t, "--dir", dir, "--today", "2026-10-28")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"scale (default, 1 tasks)", "today: 1/1 hit", "points 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}
}

func TestNonstopStats_DurationsLog(t *testing.T) {
	dir := t.TempDir()
	est := "2026-10-28 · a · hub · fix · M · M · ~9m · ~9m · 1 · 0 · bounded · T2 · -\n"
	log := "2026-10-28T09:00:00.000Z · a · work · 120000\n" +
		"2026-10-28T09:05:00.000Z · a · review · 180000\n" +
		"broken line\n"
	for name, body := range map[string]string{"estimates.md": est, "durations.log": log} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := execNonstopStats(t, "--json", "--dir", dir, "--today", "2026-10-28")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Today struct {
			ReviewMinutes int `json:"reviewMinutes"`
			Points        int `json:"points"`
		} `json:"today"`
		Week struct {
			ReviewShare float64 `json:"reviewShare"`
		} `json:"week"`
		Tiers              map[string]int `json:"tiers"`
		DurationsMalformed int            `json:"durationsMalformed"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	// log: work 2m (S), review 3m -> share 0.6; the ledger's 9m columns lose
	if got.Today.ReviewMinutes != 3 || got.Week.ReviewShare != 0.6 || got.Today.Points != 1 {
		t.Errorf("log must win: %+v", got)
	}
	if got.Tiers["T2"] != 1 || got.Tiers["T1"] != 0 || got.DurationsMalformed != 1 {
		t.Errorf("tiers/malformed: %+v", got)
	}
	human, err := execNonstopStats(t, "--dir", dir, "--today", "2026-10-28")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"review: 3m (60%)", "tiers 7d: T1 0 · T2 1 · T3 0", "durations.log malformed lines: 1"} {
		if !strings.Contains(human, want) {
			t.Errorf("human output missing %q:\n%s", want, human)
		}
	}
}

func TestNonstopStats_DurationsLogUnreadable(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "durations.log"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "estimates.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := execNonstopStats(t, "--json", "--dir", dir); err == nil {
		t.Fatal("a directory named durations.log must fail")
	}
}
