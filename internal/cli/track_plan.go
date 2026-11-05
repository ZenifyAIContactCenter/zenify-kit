package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
	"github.com/spf13/cobra"
)

func newTrackPlanCmd(ws *string) *cobra.Command {
	c := &cobra.Command{
		Use:   "plan",
		Short: "Quản lý kế hoạch trong ngày của pane", //znf:allow-lang
	}
	c.AddCommand(newTrackPlanSetCmd(ws))
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
			today := time.Now().Local().Format("2006-01-02")
			return runTrackPlanSet(w, cmd.ErrOrStderr(), trackLogPath(wsDir), today, args)
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
