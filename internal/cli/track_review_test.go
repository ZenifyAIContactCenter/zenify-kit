package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
)

// reviewWS is a workspace whose track run ended with prs, and whose merged.json holds cache.
func reviewWS(t *testing.T, prs []string, cache map[string]string) (ws, id string) {
	t.Helper()
	ws = trackWS(t, true)
	id = startTrack(t, ws, "cook")
	args := []string{"end", id, "--rounds", "0"}
	for _, p := range prs {
		args = append(args, "--pr", p)
	}
	if _, err := runTrack(t, ws, args...); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "track", "merged.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	oldM, oldT := trackMerged, trackPRTitle
	trackMerged = func(string) (string, error) { return "", nil }
	trackPRTitle = func(string) (string, error) { return "", nil }
	t.Cleanup(func() { trackMerged, trackPRTitle = oldM, oldT })
	return ws, id
}

// stubReview counts the review calls; fail makes them return an error.
func stubReview(t *testing.T, fail *bool) *[]string {
	t.Helper()
	var asked []string
	old := trackReview
	trackReview = func(u string) (trackstats.ReviewSnapshot, error) {
		asked = append(asked, u)
		if fail != nil && *fail {
			return trackstats.ReviewSnapshot{}, errors.New("gh down")
		}
		return trackstats.ReviewSnapshot{URL: u, MergedAt: "2026-11-18T05:00:00Z"}, nil
	}
	t.Cleanup(func() { trackReview = old })
	return &asked
}

func reviewStats(t *testing.T, ws string) string {
	t.Helper()
	out, err := runTrack(t, ws, "stats", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Today struct {
			MergedError string `json:"mergedError"`
		} `json:"today"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	return r.Today.MergedError
}

func reviewFile(ws, name string) string {
	return filepath.Join(ws, ".zenify", "track", "reviews", name)
}

func dropChecked(t *testing.T, ws string) {
	t.Helper()
	if err := os.Remove(filepath.Join(ws, ".zenify", "track", "merged.checked")); err != nil {
		t.Fatal(err)
	}
}

// SC-3: a merged PR is snapshotted once with its run id; a closed PR never is.
func TestTrackReview_SnapshotOncePerMergedPR(t *testing.T) {
	merged, closed := "https://github.com/o/r/pull/1", "https://github.com/o/r/pull/2"
	ws, id := reviewWS(t, []string{merged, closed}, map[string]string{merged: "2026-11-18T05:00:00Z", closed: trackstats.ClosedUnmerged})
	asked := stubReview(t, nil)
	if e := reviewStats(t, ws); e != "" {
		t.Fatalf("mergedError = %q", e)
	}
	if len(*asked) != 1 || (*asked)[0] != merged {
		t.Fatalf("asked %v, want only %s", *asked, merged)
	}
	b, err := os.ReadFile(reviewFile(ws, "o-r-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s trackstats.ReviewSnapshot
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s.TrackID != id || s.URL != merged {
		t.Errorf("snapshot trackId %q url %q, want %q %q", s.TrackID, s.URL, id, merged)
	}
	if _, err := time.Parse(time.RFC3339, s.FetchedAt); err != nil {
		t.Errorf("fetchedAt %q is not RFC 3339: %v", s.FetchedAt, err)
	}
	if _, err := os.Stat(reviewFile(ws, "o-r-2.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("closed PR got a snapshot: %v", err)
	}
	dropChecked(t, ws)
	reviewStats(t, ws)
	if len(*asked) != 1 {
		t.Fatalf("second run asked %v, want no new call", *asked)
	}
}

// SC-4: a failed snapshot is reported, writes no file and is retried next cycle.
func TestTrackReview_ErrorReportedAndRetried(t *testing.T) {
	pr := "https://github.com/o/r/pull/1"
	ws, _ := reviewWS(t, []string{pr}, map[string]string{pr: "2026-11-18T05:00:00Z"})
	fail := true
	asked := stubReview(t, &fail)
	errFile, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	oldErr := os.Stderr
	os.Stderr = errFile
	e := reviewStats(t, ws)
	os.Stderr = oldErr
	if !strings.Contains(e, "review snapshot "+pr) || !strings.Contains(e, "gh down") {
		t.Fatalf("mergedError = %q, want the review snapshot error", e)
	}
	stderr, err := os.ReadFile(errFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if want := "track: review snapshot " + pr + ": gh down"; !strings.Contains(string(stderr), want) {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if _, err := os.Stat(reviewFile(ws, "o-r-1.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed snapshot wrote a file: %v", err)
	}
	fail = false
	dropChecked(t, ws)
	if e := reviewStats(t, ws); e != "" {
		t.Fatalf("retry mergedError = %q", e)
	}
	if len(*asked) != 2 {
		t.Fatalf("asked %v, want a retry", *asked)
	}
	if _, err := os.Stat(reviewFile(ws, "o-r-1.json")); err != nil {
		t.Fatalf("retry wrote no file: %v", err)
	}
}

// SC-5: at most five snapshots per cycle.
func TestTrackReview_FivePerCycle(t *testing.T) {
	cache := map[string]string{}
	var prs []string
	for i := 1; i <= 7; i++ {
		u := fmt.Sprintf("https://github.com/o/r/pull/%d", i)
		prs = append(prs, u)
		cache[u] = "2026-11-18T05:00:00Z"
	}
	ws, _ := reviewWS(t, prs, cache)
	asked := stubReview(t, nil)
	reviewStats(t, ws)
	if len(*asked) != 5 {
		t.Fatalf("asked %d times, want 5", len(*asked))
	}
	dropChecked(t, ws)
	reviewStats(t, ws)
	if len(*asked) != 7 {
		t.Fatalf("next cycle total %d, want 7", len(*asked))
	}
}

// Inside the recheck window stats never asks gh for a review.
func TestTrackReview_NoCallInsideRecheckWindow(t *testing.T) {
	pr := "https://github.com/o/r/pull/1"
	ws, _ := reviewWS(t, []string{pr}, map[string]string{pr: "2026-11-18T05:00:00Z"})
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "track", "merged.checked"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	asked := stubReview(t, nil)
	reviewStats(t, ws)
	if len(*asked) != 0 {
		t.Fatalf("asked %v inside the recheck window", *asked)
	}
}
