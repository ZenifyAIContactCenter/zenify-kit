package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
	"github.com/spf13/cobra"
)

func newTrackPlanCmd(ws *string) *cobra.Command {
	c := &cobra.Command{
		Use:   "plan",
		Short: "Quản lý kế hoạch trong ngày của pane", //znf:allow-lang
	}
	c.AddCommand(newTrackPlanSetCmd(ws), newTrackPlanEditCmd(ws, true), newTrackPlanEditCmd(ws, false))
	return c
}

func newTrackPlanSetCmd(ws *string) *cobra.Command {
	return &cobra.Command{
		Use:   "set <id>...",
		Short: "Đặt kế hoạch hôm nay từ các id backlog, SDD, track hoặc URL", //znf:allow-lang
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			wsDir, on, err := trackWorkspace(*ws)
			if err != nil {
				return err
			}
			if !on {
				_, err = fmt.Fprintln(w, "track: off")
				return err
			}
			now := time.Now()
			ids := make([]string, 0, len(args))
			for _, a := range args {
				id, err := resolveRef(filepath.Dir(trackLogPath(wsDir)), wsDir, a, scopePlanAll, now)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return runTrackPlanSet(w, cmd.ErrOrStderr(), trackLogPath(wsDir), now.Local().Format("2006-01-02"), ids)
		},
	}
}

// runTrackPlanSet appends one `plan` row holding the distinct ids, in order.
func runTrackPlanSet(w, errW io.Writer, logPath, today string, ids []string) error {
	if len(ids) == 0 {
		return badTrack("expected at least 1 id")
	}
	seen := map[string]bool{}
	uniq := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.ContainsAny(id, "\r\n") || strings.Contains(id, trackSep) || !trackstats.ValidPlanID(id) {
			return badTrack("invalid plan id %q", id)
		}
		if !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	unlock, err := lockTrackLog(logPath)
	if err != nil {
		return err
	}
	defer releaseTrackLock(errW, logPath, unlock)
	if err := appendTrackRow(logPath, "plan", append([]string{today}, uniq...)); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "track: plan %d items\n", len(uniq))
	return err
}

// newTrackPlanEditCmd builds `plan add` (add=true) or `plan rm`.
func newTrackPlanEditCmd(ws *string, add bool) *cobra.Command {
	use, short, scope := "rm <ref>...", "Bỏ mục khỏi kế hoạch hôm nay", scopePlanToday //znf:allow-lang
	if add {
		use, short, scope = "add <ref>...", "Thêm mục vào kế hoạch hôm nay", scopePlanAll //znf:allow-lang
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			wsDir, on, err := trackWorkspace(*ws)
			if err != nil {
				return err
			}
			if !on {
				_, err = fmt.Fprintln(w, "track: off")
				return err
			}
			now := time.Now()
			ids := make([]string, 0, len(args))
			for _, a := range args {
				id, err := resolveRef(filepath.Dir(trackLogPath(wsDir)), wsDir, a, scope, now)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return runTrackPlanEdit(w, cmd.ErrOrStderr(), trackLogPath(wsDir), now.Local().Format("2006-01-02"), ids, add)
		},
	}
}

// todayPlanRow returns the ids of the latest `plan` row dated today, empty when there is none.
func todayPlanRow(logPath, today string) ([]string, error) {
	b, err := os.ReadFile(logPath) //nolint:gosec // G304 -- computed from the resolved workspace
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, exitcode.New(exitcode.Fail, err)
	}
	ev, _ := trackstats.ParseLog(string(b))
	var ids []string
	for _, e := range ev {
		if e.Type == "plan" && e.ID == today {
			ids = e.F
		}
	}
	return ids, nil
}

// runTrackPlanEdit appends a new full-list `plan` row: today's plan plus ids (add) or minus ids (rm).
func runTrackPlanEdit(w, errW io.Writer, logPath, today string, ids []string, add bool) error {
	for _, id := range ids {
		if strings.ContainsAny(id, "\r\n") || strings.Contains(id, trackSep) || !trackstats.ValidPlanID(id) {
			return badTrack("invalid plan id %q", id)
		}
	}
	unlock, err := lockTrackLog(logPath)
	if err != nil {
		return err
	}
	defer releaseTrackLock(errW, logPath, unlock)
	cur, err := todayPlanRow(logPath, today)
	if err != nil {
		return err
	}
	in := map[string]bool{}
	for _, id := range cur {
		in[id] = true
	}
	next := append([]string(nil), cur...)
	if add {
		for _, id := range ids {
			if !in[id] {
				in[id] = true
				next = append(next, id)
			}
		}
	} else {
		drop := map[string]bool{}
		for _, id := range ids {
			if !in[id] {
				return badTrack("%s is not in today's plan", id)
			}
			drop[id] = true
		}
		next = next[:0]
		for _, id := range cur {
			if !drop[id] {
				next = append(next, id)
			}
		}
	}
	if err := appendTrackRow(logPath, "plan", append([]string{today}, next...)); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "track: plan %d items\n", len(next))
	return err
}
