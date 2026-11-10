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
			return runCloseDay(cmd.OutOrStdout(), cmd.ErrOrStderr(), wsDir, time.Now())
		},
	}
}

// runCloseDay closes the previous working day once per calendar day: report, archive, mark.
func runCloseDay(w, errW io.Writer, wsDir string, now time.Time) error {
	dir := filepath.Join(wsDir, ".zenify", "track")
	today := now.Format("2006-01-02")
	closedPath := filepath.Join(dir, "day.closed")
	if b, err := os.ReadFile(closedPath); err == nil && strings.TrimSpace(string(b)) == today { //nolint:gosec // G304 -- computed from the resolved workspace
		return nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Force a fresh gh check; no lock is held here because stats may call gh.
	if err := os.Remove(filepath.Join(dir, "merged.checked")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	nowRes, err := computeTrackStats(dir, wsDir, today, now, false)
	if err != nil {
		return err
	}
	report := ""
	if nowRes.Kitchen.LastDay != nil {
		d := nowRes.Kitchen.LastDay.Date
		dayRes, err := computeTrackStats(dir, wsDir, d, now, false)
		if err != nil {
			return err
		}
		prNote := ""
		if e := firstNonEmpty(dayRes.Today.MergedError, dayRes.Today.PRStatusError); e != "" {
			prNote = "PR status may be stale: " + e
		}
		cache, err := readMergedCache(dir)
		if err != nil {
			return err
		}
		report = trackstats.DayReport(dayRes, nowRes, cache, prNote)
		if err := os.MkdirAll(filepath.Join(dir, "days"), 0o750); err != nil {
			return err
		}
		if err := writeFileAtomic(filepath.Join(dir, "days", d+".md"), []byte(report), 0o644); err != nil {
			return err
		}
	}

	keep, movedIDs, err := compactTrackLog(errW, dir, now, today)
	if err != nil {
		return err
	}
	if err := moveOldFiles(dir, now, keep, movedIDs); err != nil {
		return err
	}
	if err := writeFileAtomic(closedPath, []byte(today+"\n"), 0o644); err != nil {
		return err
	}
	if report != "" {
		_, err = io.WriteString(w, report)
	}
	return err
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
		if err != nil {
			return nil, err
		}
		out[strings.TrimSuffix(filepath.Base(f), ".log")] = string(b)
	}
	return out, nil
}

// compactTrackLog archives the old rows under the log lock: archive first, then the log
// rewrite, so a failure between them leaves duplicates in archive, never a lost row.
func compactTrackLog(errW io.Writer, dir string, now time.Time, today string) (string, map[string]bool, error) {
	logPath := filepath.Join(dir, "log.md")
	unlock, err := lockTrackLog(logPath)
	if err != nil {
		return "", nil, err
	}
	defer releaseTrackLock(errW, logPath, unlock)
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
		if t, err := time.Parse(time.RFC3339, strings.SplitN(line, trackSep, 2)[0]); err == nil {
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
// A session log moves when it is old and none of its bound ids is still in the log.
func moveOldFiles(dir string, now time.Time, keep string, movedIDs map[string]bool) error {
	cut := now.Add(-closeDayKeepDays * 24 * time.Hour)
	files, err := filepath.Glob(filepath.Join(dir, "sessions", "*.log"))
	if err != nil {
		return err
	}
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil {
			return err
		}
		if !fi.ModTime().Before(cut) {
			continue
		}
		b, err := os.ReadFile(f) //nolint:gosec // G304 -- listed from the track dir
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
		if strings.TrimSuffix(filepath.Base(f), ".md") < cutDay {
			if err := moveInto(f, filepath.Join(dir, "archive", "days")); err != nil {
				return err
			}
		}
	}
	return nil
}

func moveInto(src, destDir string) error {
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return err
	}
	return os.Rename(src, filepath.Join(destDir, filepath.Base(src)))
}
