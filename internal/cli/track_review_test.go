package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	e, _ := reviewStatsErr(t, ws)
	return e
}

// reviewStatsErr runs stats --json and returns Today.MergedError and what went to stderr.
func reviewStatsErr(t *testing.T, ws string) (mergedErr, stderr string) {
	t.Helper()
	root := NewRootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"track", "stats", "--json", "--workspace", ws})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var r struct {
		Today struct {
			MergedError string `json:"mergedError"`
		} `json:"today"`
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r.Today.MergedError, errOut.String()
}

// mergedAgo is a merged.json value for a merge d before now.
func mergedAgo(d time.Duration) string {
	return time.Now().Add(-d).UTC().Format(time.RFC3339)
}

func readFailed(t *testing.T, ws string) map[string]int {
	t.Helper()
	m := map[string]int{}
	b, err := os.ReadFile(filepath.Join(ws, ".zenify", "track", "reviews.failed"))
	if errors.Is(err, os.ErrNotExist) {
		return m
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
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
	ws, id := reviewWS(t, []string{merged, closed}, map[string]string{merged: mergedAgo(48 * time.Hour), closed: trackstats.ClosedUnmerged})
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

// SC-4: a failed snapshot goes to stderr, not onto mergedError, writes no file, is counted in
// reviews.failed and is retried next cycle; a success clears its count.
func TestTrackReview_ErrorReportedAndRetried(t *testing.T) {
	pr := "https://github.com/o/r/pull/1"
	ws, _ := reviewWS(t, []string{pr}, map[string]string{pr: mergedAgo(48 * time.Hour)})
	fail := true
	asked := stubReview(t, &fail)
	e, stderr := reviewStatsErr(t, ws)
	if e != "" {
		t.Fatalf("mergedError = %q, want empty: a snapshot failure is not a merged-check failure", e)
	}
	if want := "track: review snapshot " + pr + ": gh down"; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if _, err := os.Stat(reviewFile(ws, "o-r-1.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed snapshot wrote a file: %v", err)
	}
	if got := readFailed(t, ws); got[pr] != 1 {
		t.Fatalf("reviews.failed = %v, want %s: 1", got, pr)
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
	if got := readFailed(t, ws); len(got) != 0 {
		t.Fatalf("reviews.failed = %v after success, want empty", got)
	}
}

// Only the first failure of a cycle reaches stderr.
func TestTrackReview_FirstFailureOnlyOnStderr(t *testing.T) {
	a, b := "https://github.com/o/r/pull/1", "https://github.com/o/r/pull/2"
	ws, _ := reviewWS(t, []string{a, b}, map[string]string{a: mergedAgo(time.Hour), b: mergedAgo(2 * time.Hour)})
	fail := true
	stubReview(t, &fail)
	_, stderr := reviewStatsErr(t, ws)
	if n := strings.Count(stderr, "track: review snapshot "); n != 1 {
		t.Fatalf("stderr has %d snapshot lines, want 1:\n%s", n, stderr)
	}
	if got := readFailed(t, ws); got[a] != 1 || got[b] != 1 {
		t.Fatalf("reviews.failed = %v, want both counted", got)
	}
}

// A PR that fails three cycles stops taking a slot; an older PR gets it.
func TestTrackReview_GivesUpAfterThreeFailures(t *testing.T) {
	cache := map[string]string{}
	var prs []string
	for i := 1; i <= 6; i++ {
		u := fmt.Sprintf("https://github.com/o/r/pull/%d", i)
		prs = append(prs, u)
		cache[u] = mergedAgo(time.Duration(10-i) * time.Hour)
	}
	ws, _ := reviewWS(t, prs, cache)
	fail := true
	asked := stubReview(t, &fail)
	for cycle := 1; cycle <= 3; cycle++ {
		*asked = nil
		reviewStatsErr(t, ws)
		if len(*asked) != 5 || slices.Contains(*asked, prs[0]) {
			t.Fatalf("cycle %d asked %v, want the 5 newest", cycle, *asked)
		}
		dropChecked(t, ws)
	}
	*asked = nil
	reviewStatsErr(t, ws)
	if len(*asked) != 1 || (*asked)[0] != prs[0] {
		t.Fatalf("cycle 4 asked %v, want only the oldest %s", *asked, prs[0])
	}
}

// A PR merged before the keep window is never fetched.
func TestTrackReview_OnlyRecentMerges(t *testing.T) {
	old, recent := "https://github.com/o/r/pull/1", "https://github.com/o/r/pull/2"
	ws, _ := reviewWS(t, []string{old, recent}, map[string]string{
		old: mergedAgo((closeDayKeepDays + 1) * 24 * time.Hour), recent: mergedAgo(time.Hour),
	})
	asked := stubReview(t, nil)
	reviewStats(t, ws)
	if len(*asked) != 1 || (*asked)[0] != recent {
		t.Fatalf("asked %v, want only %s", *asked, recent)
	}
}

// SC-5: at most five snapshots per cycle.
func TestTrackReview_FivePerCycle(t *testing.T) {
	cache := map[string]string{}
	var prs []string
	for i := 1; i <= 7; i++ {
		u := fmt.Sprintf("https://github.com/o/r/pull/%d", i)
		prs = append(prs, u)
		cache[u] = mergedAgo(48 * time.Hour)
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

// A new merge gets a slot ahead of old PRs that always fail: candidates go newest merge first.
func TestTrackReview_NewestMergeFirst(t *testing.T) {
	cache := map[string]string{}
	var prs []string
	for i := 1; i <= 6; i++ {
		u := fmt.Sprintf("https://github.com/o/r/pull/%d", i)
		prs = append(prs, u)
		cache[u] = mergedAgo(time.Duration(10-i) * time.Hour)
	}
	newest := prs[5]
	ws, _ := reviewWS(t, prs, cache)
	var asked []string
	old := trackReview
	trackReview = func(u string) (trackstats.ReviewSnapshot, error) {
		asked = append(asked, u)
		if u != newest {
			return trackstats.ReviewSnapshot{}, errors.New("no pullRequest in the answer")
		}
		return trackstats.ReviewSnapshot{URL: u}, nil
	}
	t.Cleanup(func() { trackReview = old })
	reviewStatsErr(t, ws)
	if len(asked) != 5 || asked[0] != newest {
		t.Fatalf("asked %v, want 5 calls starting with %s", asked, newest)
	}
	if _, err := os.Stat(reviewFile(ws, "o-r-6.json")); err != nil {
		t.Fatalf("newest merge has no snapshot after one cycle: %v", err)
	}
}

// Inside the recheck window stats never asks gh for a review.
func TestTrackReview_NoCallInsideRecheckWindow(t *testing.T) {
	pr := "https://github.com/o/r/pull/1"
	ws, _ := reviewWS(t, []string{pr}, map[string]string{pr: mergedAgo(48 * time.Hour)})
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "track", "merged.checked"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	asked := stubReview(t, nil)
	reviewStats(t, ws)
	if len(*asked) != 0 {
		t.Fatalf("asked %v inside the recheck window", *asked)
	}
}
