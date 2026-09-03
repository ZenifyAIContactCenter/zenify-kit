package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/observe"
	"github.com/spf13/cobra"
)

var reportNow = time.Now // seam for tests

// humanAge renders a compact relative age; zero time → "—".
func humanAge(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// runObserveReport prints the per-session observe summary. Factored to take the
// lister so tests inject sessions without touching the filesystem. Unlike the
// hook commands this is human-invoked, so it returns a real error on failure
// rather than swallowing it.
func runObserveReport(w io.Writer, asJSON bool, list func() ([]observe.SessionSummary, error)) error {
	sessions, err := list()
	if err != nil {
		return err
	}

	if asJSON {
		b, err := json.MarshalIndent(sessions, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(w, string(b))
		return nil
	}

	if len(sessions) == 0 {
		_, _ = fmt.Fprintln(w, "No observe data yet. (Hooks record into $XDG_STATE_HOME/zenify/observe.)")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SESSION\tDISPATCH\tCALLS\tTOOL-OUT\tLAST-ACTIVE")
	now := reportNow()
	for _, s := range sessions {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%s\n",
			s.ID, s.Count, s.Calls, humanBytes(s.Bytes), humanAge(s.ModTime, now))
	}
	return tw.Flush()
}

const reportLong = `Summarize this kit's observe state per session: subagent dispatches (from
` + "`zenify observe count`" + `) and tool-output volume (from ` + "`zenify observe meter`" + `),
newest-active first. This is the in-stack, single-binary alternative to a
heavyweight web dashboard.

For a full real-time web dashboard (multi-agent replay, filtering, token graphs)
run simple10/agents-observe alongside — it is MIT and registers its own Claude
Code hooks, so it coexists with the znf hooks rather than replacing them.`

func newReportCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "report",
		Short: "Tóm tắt observe theo session (dispatch + tool-output)",
		Long:  reportLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runObserveReport(cmd.OutOrStdout(), asJSON, observe.ListSessions)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "output JSON instead of a table")
	return c
}
