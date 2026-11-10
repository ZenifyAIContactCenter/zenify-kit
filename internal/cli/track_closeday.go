package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
)

// closeDayKeepDays is how long log.md rows, session logs and day reports stay live.
const closeDayKeepDays = 14

func newTrackCloseDayCmd(ws *string) *cobra.Command {
	return &cobra.Command{
		Use:   "close-day",
		Short: "Chốt ngày làm việc trước: báo cáo ngày và archive log cũ", //znf:allow-lang
		Long: "Lần chạy đầu tiên trong ngày ghi báo cáo ngày làm việc gần nhất vào days/<ngày>.md,\n" + //znf:allow-lang
			"chuyển các dòng log.md và session log cũ hơn 14 ngày sang archive/ rồi in báo cáo.\n" + //znf:allow-lang
			"Các lần sau trong cùng ngày không làm gì.", //znf:allow-lang
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			wsDir, on, err := trackWorkspace(*ws)
			if err != nil {
				return err
			}
			if !on {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "track: off")
				return err
			}
			return runCloseDay(cmd.OutOrStdout(), cmd.ErrOrStderr(), wsDir, time.Now(), true)
		},
	}
}

// runCloseDay closes the previous working day once per calendar day: report, archive, mark.
// refresh asks gh for fresh PR state (the manual command); without it the report reads the
// cached PR state from local files only, so SessionStart makes no gh call.
func runCloseDay(w, errW io.Writer, wsDir string, now time.Time, refresh bool) error {
	dir := filepath.Join(wsDir, ".zenify", "track")
	today := now.Format("2006-01-02")
	closedPath := filepath.Join(dir, "day.closed")
	if closed, err := dayClosed(closedPath, today); err != nil || closed {
		return err
	}

	var (
		in     trackstats.Input
		nowRes trackstats.Result
		prNote string
		err    error
	)
	if refresh {
		// Force a fresh gh check; no lock is held here because stats may call gh.
		if err := os.Remove(filepath.Join(dir, "merged.checked")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if nowRes, in, err = computeTrackStatsInput(dir, wsDir, today, now, false); err != nil {
			return err
		}
		if e := firstNonEmpty(nowRes.Today.MergedError, nowRes.Today.PRStatusError); e != "" {
			prNote = "PR status may be stale: " + e
		}
	} else {
		in, err = localTrackInput(dir, wsDir, now)
		if errors.Is(err, errNoTrackLog) {
			return nil
		}
		if err != nil {
			return err
		}
		nowRes, _ = trackstats.Compute(in)
		if nowRes.Today.PRsWaiting > 0 || len(nowRes.Today.PROpen) > 0 {
			prNote = "PR status from cache"
		}
	}
	report, ownDay := "", ""
	if nowRes.Kitchen.LastDay != nil {
		ownDay = nowRes.Kitchen.LastDay.Date
		// The day's figures rerun Compute on the same Input: no second file read, no gh.
		dayIn := in
		dayIn.Today = ownDay
		dayIn.Merged, dayIn.PRStatus, dayIn.PRTitle, dayIn.GitHub, dayIn.SDD = nil, nil, nil, nil, nil
		dayRes, _ := trackstats.Compute(dayIn)
		report = trackstats.DayReport(dayRes, nowRes, in.Cache, prNote)
		if err := os.MkdirAll(filepath.Join(dir, "days"), 0o750); err != nil {
			return err
		}
		if err := writeFileAtomic(filepath.Join(dir, "days", ownDay+".md"), []byte(report), 0o644); err != nil {
			return err
		}
	}

	logPath := filepath.Join(dir, "log.md")
	unlock, err := lockTrackLog(logPath)
	if err != nil {
		return err
	}
	err = closeDayLocked(dir, closedPath, now, today, ownDay)
	releaseTrackLock(errW, logPath, unlock)
	if err != nil {
		return err
	}
	if report != "" {
		_, err = io.WriteString(w, report)
	}
	return err
}

// dayClosed reports whether day.closed already holds today.
func dayClosed(closedPath, today string) (bool, error) {
	b, err := os.ReadFile(closedPath) //nolint:gosec // G304 -- computed from the resolved workspace
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(b)) == today, nil
}

// closeDayLocked compacts log.md, moves old files and marks the day; the caller holds the log
// lock. A run that finds the day already closed by a parallel run does nothing.
func closeDayLocked(dir, closedPath string, now time.Time, today, ownDay string) error {
	if closed, err := dayClosed(closedPath, today); err != nil || closed {
		return err
	}
	keep, movedIDs, err := compactTrackLog(dir, now, today)
	if err != nil {
		return err
	}
	if err := moveOldFiles(dir, now, keep, movedIDs, ownDay); err != nil {
		return err
	}
	return writeFileAtomic(closedPath, []byte(today+"\n"), 0o644)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func readMergedCache(dir string) (map[string]string, error) {
	cache := map[string]string{}
	b, err := os.ReadFile(filepath.Join(dir, "merged.json")) //nolint:gosec // G304 -- computed from the resolved workspace
	if errors.Is(err, os.ErrNotExist) {
		return cache, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &cache); err != nil {
		return nil, fmt.Errorf("parse merged.json: %w", err)
	}
	return cache, nil
}

func readSessionLogs(dir string) (map[string]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "sessions", "*.log"))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // G304 -- listed from the track dir
		if errors.Is(err, os.ErrNotExist) {
			continue // archived by a parallel close-day between the glob and the read
		}
		if err != nil {
			return nil, err
		}
		out[strings.TrimSuffix(filepath.Base(f), ".log")] = string(b)
	}
	return out, nil
}

// compactTrackLog archives the old rows; the caller holds the log lock. Archive first, then the
// log rewrite, so a failure between them leaves duplicates in archive, never a lost row.
func compactTrackLog(dir string, now time.Time, today string) (string, map[string]bool, error) {
	logPath := filepath.Join(dir, "log.md")
	b, err := os.ReadFile(logPath) //nolint:gosec // G304 -- computed from the resolved workspace
	if errors.Is(err, os.ErrNotExist) {
		return "", map[string]bool{}, nil
	}
	if err != nil {
		return "", nil, err
	}
	sessions, err := readSessionLogs(dir)
	if err != nil {
		return "", nil, err
	}
	cache, err := readMergedCache(dir)
	if err != nil {
		return "", nil, err
	}
	keep, moved, movedIDs := trackstats.Compact(trackstats.Input{Log: string(b), Sessions: sessions, Cache: cache, Now: now, Today: today}, closeDayKeepDays)
	if len(moved) == 0 {
		return keep, movedIDs, nil
	}
	byMonth := map[string][]string{}
	for _, line := range moved {
		month := today[:7]
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(strings.SplitN(line, trackSep, 2)[0])); err == nil {
			month = t.In(now.Location()).Format("2006-01")
		}
		byMonth[month] = append(byMonth[month], line)
	}
	if err := os.MkdirAll(filepath.Join(dir, "archive"), 0o750); err != nil {
		return "", nil, err
	}
	for month, lines := range byMonth {
		f, err := os.OpenFile(filepath.Join(dir, "archive", month+".md"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // G304 -- computed from the resolved workspace
		if err != nil {
			return "", nil, err
		}
		_, werr := f.WriteString(strings.Join(lines, "\n") + "\n")
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return "", nil, werr
		}
	}
	if err := writeFileAtomic(logPath, []byte(keep), 0o644); err != nil {
		return "", nil, err
	}
	return keep, movedIDs, nil
}

// moveOldFiles moves session logs and day reports past the keep window into archive/.
// A session log moves when it is old and none of its bound ids is still in the log; ownDay's
// report, written by this run, always stays. A file already gone was moved by another run.
func moveOldFiles(dir string, now time.Time, keep string, movedIDs map[string]bool, ownDay string) error {
	cut := now.Add(-closeDayKeepDays * 24 * time.Hour)
	files, err := filepath.Glob(filepath.Join(dir, "sessions", "*.log"))
	if err != nil {
		return err
	}
	for _, f := range files {
		fi, err := os.Stat(f)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !fi.ModTime().Before(cut) {
			continue
		}
		b, err := os.ReadFile(f) //nolint:gosec // G304 -- listed from the track dir
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		evs, _, _ := trackstats.ParseSession("", string(b))
		movable := true
		for _, e := range evs {
			if e.Type == "bind" && !movedIDs[e.ID] && strings.Contains(keep, e.ID) {
				movable = false
				break
			}
		}
		if movable {
			if err := moveInto(f, filepath.Join(dir, "archive", "sessions")); err != nil {
				return err
			}
		}
	}
	days, err := filepath.Glob(filepath.Join(dir, "days", "*.md"))
	if err != nil {
		return err
	}
	cutDay := cut.Format("2006-01-02")
	for _, f := range days {
		if d := strings.TrimSuffix(filepath.Base(f), ".md"); d < cutDay && d != ownDay {
			if err := moveInto(f, filepath.Join(dir, "archive", "days")); err != nil {
				return err
			}
		}
	}
	return nil
}

// moveInto renames src into destDir; an existing file of that name is never overwritten,
// the new one gets a numeric suffix instead. A src already gone counts as moved.
func moveInto(src, destDir string) error {
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return err
	}
	dst := filepath.Join(destDir, filepath.Base(src))
	for i := 1; ; i++ {
		_, err := os.Lstat(dst)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return err
		}
		dst = filepath.Join(destDir, fmt.Sprintf("%s.%d", filepath.Base(src), i))
	}
	if err := os.Rename(src, dst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
