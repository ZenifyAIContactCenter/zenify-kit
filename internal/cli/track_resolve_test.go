package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
)

// seedGhCache writes a fresh backlog-gh.json so stats reads these issues without a gh search.
func seedGhCache(t *testing.T, ws string, issues ...trackstats.Issue) {
	t.Helper()
	dir := filepath.Join(ws, ".zenify", "track")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(backlogCacheFile{FetchedAt: time.Now().UTC(), Issues: issues})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, backlogCache), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveFullIDPassthrough(t *testing.T) {
	noGh(t)
	ws := trackWS(t, true)
	// A fresh gh cache keeps backlogSources from starting a search that outlives the failed run.
	seedGhCache(t, ws)
	dir := filepath.Join(ws, ".zenify", "track")
	// log.md is a directory, so any stats run fails: a full id must not reach it.
	if err := os.MkdirAll(filepath.Join(dir, "log.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	bid := "b-261111-b81e"
	got, err := resolveRef(dir, ws, bid, scopeBacklog, time.Now())
	if err != nil || got != bid {
		t.Fatalf("resolveRef(full id) = %q %v", got, err)
	}
	if _, err := resolveRef(dir, ws, "b81e", scopeBacklog, time.Now()); err == nil {
		t.Fatal("a short code must read stats, which fail here")
	}
}

// SC-2
func TestTrackStartFromAmbiguous(t *testing.T) {
	noGh(t)
	ws := trackWS(t, true)
	infra := "https://github.com/Z/infrastructure/issues/5"
	dash := "https://github.com/Z/zenify-dashboard/issues/5"
	seedGhCache(t, ws,
		trackstats.Issue{URL: infra, Repo: "infrastructure", Title: "infra five", CreatedAt: time.Now()},
		trackstats.Issue{URL: dash, Repo: "zenify-dashboard", Title: "dash five", CreatedAt: time.Now()},
	)
	before := trackLog(t, ws)
	_, err := runTrack(t, ws, "start", "--kind", "cook", "--repo", "infrastructure", "--from", "#5")
	if err == nil {
		t.Fatal("ambiguous --from must fail")
	}
	msg := err.Error()
	for _, want := range []string{"track: #5 matches 2 items:", "  #5 · infrastructure · infra five", "  #5 · zenify-dashboard · dash five", "use <repo>#<n> or the full id"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q lacks %q", msg, want)
		}
	}
	if trackLog(t, ws) != before {
		t.Fatalf("log changed: %q", trackLog(t, ws))
	}
	if _, err := runTrack(t, ws, "start", "--kind", "cook", "--repo", "infrastructure", "--from", "infrastructure#5"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(trackLog(t, ws), " · cook · infrastructure · - · "+infra+"\n") {
		t.Fatalf("start row = %q", trackLog(t, ws))
	}
}

// SC-3
func TestTrackPlanSetNoMatch(t *testing.T) {
	noGh(t)
	ws := trackWS(t, true)
	addBacklog(t, ws, "x", "--repo", "kit")
	before := trackLog(t, ws)
	_, err := runTrack(t, ws, "plan", "set", "zzzz")
	if err == nil || err.Error() != "track: no item matches zzzz" {
		t.Fatalf("err = %v", err)
	}
	if trackLog(t, ws) != before {
		t.Fatalf("log changed: %q", trackLog(t, ws))
	}
}

func TestTrackPlanSetResolvesCode(t *testing.T) {
	noGh(t)
	ws := trackWS(t, true)
	bid := addBacklog(t, ws, "x", "--repo", "kit")
	if _, err := runTrack(t, ws, "plan", "set", bid[len(bid)-4:]); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(trackLog(t, ws), " · plan · "+time.Now().Local().Format("2006-01-02")+" · "+bid+"\n") {
		t.Fatalf("log = %q", trackLog(t, ws))
	}
}

// SC-8
func TestTrackDropGithubURL(t *testing.T) {
	noGh(t)
	ws := trackWS(t, true)
	u := "https://github.com/Z/zenify-kit/issues/581"
	seedGhCache(t, ws, trackstats.Issue{URL: u, Repo: "zenify-kit", Title: "kit issue", CreatedAt: time.Now()})
	// A URL not in the backlog is refused.
	if _, err := runTrack(t, ws, "backlog", "drop", "https://github.com/Z/zenify-kit/issues/999"); err == nil || !strings.Contains(err.Error(), "unknown backlog item") {
		t.Fatalf("unknown URL drop: %v", err)
	}
	out, err := runTrack(t, ws, "backlog", "drop", "#581")
	if err != nil || out != "track: dropped "+u+"\n" {
		t.Fatalf("drop: %q %v", out, err)
	}
	if !strings.HasSuffix(trackLog(t, ws), " · drop · "+u+"\n") {
		t.Fatalf("log = %q", trackLog(t, ws))
	}
	for _, it := range trackBacklogStats(t, ws).Backlog.Items {
		if it.ID == u {
			t.Fatalf("dropped issue still in backlog: %+v", it)
		}
	}
}
