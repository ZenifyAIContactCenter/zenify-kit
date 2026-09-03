package observe

import (
	"os"
	"sort"
	"time"
)

// SessionSummary is one session's observe totals for the report command.
type SessionSummary struct {
	ID      string    `json:"id"`
	Count   int       `json:"dispatches"`   // subagent dispatches (count.json)
	Calls   int       `json:"tool_calls"`   // metered tool calls (meter.json)
	Bytes   int64     `json:"tool_bytes"`   // total tool-output bytes (meter.json)
	ModTime time.Time `json:"last_active"`  // newest of count.json / meter.json mtime
}

// ListSessions enumerates every session dir under the observe state dir and
// returns a summary per session, newest-active first. Read-only, no lock — a
// snapshot. A session whose files are absent contributes zeros; a corrupt file
// is treated as zero for that half rather than dropping the session.
func ListSessions() ([]SessionSummary, error) {
	base, err := dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no state yet — not an error
		}
		return nil, err
	}

	var out []SessionSummary
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		st, _ := readState(sessPath(base, id))
		m, _ := readMeter(meterPath(base, id))

		var mod time.Time
		for _, p := range []string{sessPath(base, id), meterPath(base, id)} {
			if fi, err := os.Stat(p); err == nil && fi.ModTime().After(mod) {
				mod = fi.ModTime()
			}
		}

		out = append(out, SessionSummary{
			ID:      id,
			Count:   st.Count,
			Calls:   m.TotalCalls(),
			Bytes:   m.TotalBytes(),
			ModTime: mod,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}
