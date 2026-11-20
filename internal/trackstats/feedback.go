package trackstats

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// FeedbackInput is what the close-day Feedback section reads; the caller does the file I/O.
type FeedbackInput struct {
	Day       string                     // YYYY-MM-DD
	Merged    []string                   // PR urls merged on Day (MergedOn), sorted
	Events    []Event                    // ParseLog of log.md
	Items     []Item                     // the day's Result.Today.Items, for est and wall
	Scale     Scale                      // the scale estimate misses are judged on
	Ledgers   map[string]string          // track id -> ledger text; missing id = no ledger
	Snapshots map[string]*ReviewSnapshot // url -> snapshot; missing url = no data
}

var severityRe = regexp.MustCompile(`\b(CRITICAL|HIGH|MEDIUM|LOW)\b`)

// ledgerMarks are the ledger line markers the Feedback counts, in output order.
var ledgerMarks = []struct{ mark, label, signal string }{
	{"fix round", "fix rounds", ""},
	{"parked —", "parked", "parked"},
	{"Ruling:", "rulings", "ruling"},
	{"minor (deferred)", "deferred", ""},
}

// Feedback renders the "## Feedback" section of the day close report: one block of figures per
// PR merged on in.Day, with fixed signal prefixes (escaped:, estimate miss:, parked:, ruling:)
// an agent can pick up. It is "" when no PR merged that day.
func Feedback(in FeedbackInput) string {
	if len(in.Merged) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Feedback\n")
	for _, u := range in.Merged {
		feedbackBlock(&b, in, u)
	}
	return b.String()
}

func feedbackBlock(b *strings.Builder, in FeedbackInput, prURL string) {
	fmt.Fprintf(b, "\n### %s\n\n", prURL)
	id := OwnerRun(in.Events, prURL)
	var signals []string

	if id == "-" {
		b.WriteString("- run: -\n")
	} else if it, ok := feedbackItem(in, id); ok {
		fmt.Fprintf(b, "- run: %s %s est %s wall %dm\n", id, it.Kind, it.Est, it.WallMin)
		if estimateScored(it) {
			if size := sizeOf(it.WallMin, in.Scale); size != it.Est {
				signals = append(signals, fmt.Sprintf("estimate miss: est %s, wall %dm is %s", it.Est, it.WallMin, size))
			}
		}
	} else {
		// The run ended on an earlier day than the merge: its figures come from the log.
		it := runFromEvents(in.Events, id)
		if it.WallMin > 0 {
			fmt.Fprintf(b, "- run: %s %s est %s wall %dm\n", id, it.Kind, it.Est, it.WallMin)
		} else {
			fmt.Fprintf(b, "- run: %s %s est %s\n", id, it.Kind, it.Est)
		}
		if estimateScored(it) {
			if size := sizeOf(it.WallMin, in.Scale); size != it.Est {
				signals = append(signals, fmt.Sprintf("estimate miss: est %s, wall %dm is %s", it.Est, it.WallMin, size))
			}
		}
	}

	ledger, hasLedger := in.Ledgers[id]
	// The id names a file path: only a real track id may, whatever the caller put in Ledgers.
	if hasLedger && idRe.MatchString(id) {
		fmt.Fprintf(b, "- ledger: .zenify/track/ledgers/%s.md\n", id)
	} else {
		b.WriteString("- ledger: none\n")
		hasLedger = false
	}
	snap := in.Snapshots[prURL]
	file, okName := ReviewFileName(prURL)
	if snap != nil && okName {
		fmt.Fprintf(b, "- review data: .zenify/track/reviews/%s\n", file)
	} else {
		// The expected path, so a reader can check later whether the snapshot has arrived.
		if okName {
			fmt.Fprintf(b, "- review data: missing (.zenify/track/reviews/%s)\n", file)
		} else {
			b.WriteString("- review data: missing\n")
		}
		snap = nil
	}

	// The last ship row of the run is the internal review the PR went out with.
	findings := "-"
	if ship := lastShip(in.Events, id); ship != nil && ship[2] != "-" {
		findings = ship[2]
		fmt.Fprintf(b, "- internal review: %s rounds %s\n", findings, ship[3])
	} else {
		b.WriteString("- internal review: -\n")
	}

	if hasLedger {
		counts := make([]int, len(ledgerMarks))
		first := make([]string, len(ledgerMarks))
		for _, l := range strings.Split(ledger, "\n") {
			for i, m := range ledgerMarks {
				if strings.Contains(l, m.mark) {
					counts[i]++
					if first[i] == "" {
						first[i] = oneLine(l)
					}
				}
			}
		}
		parts := make([]string, len(ledgerMarks))
		for i, m := range ledgerMarks {
			parts[i] = fmt.Sprintf("%s %d", m.label, counts[i])
			if m.signal != "" && counts[i] > 0 {
				signals = append(signals, fmt.Sprintf("%s: %d in the ledger, first: %s", m.signal, counts[i], first[i]))
			}
		}
		b.WriteString("- ledger counts: " + strings.Join(parts, ", ") + "\n")
	}

	if snap != nil {
		sev := map[string]int{}
		declined, human := 0, 0
		for _, th := range snap.Threads {
			if len(th.Comments) == 0 {
				continue
			}
			// Only a thread a bot opened is bot review; a person's thread is counted apart.
			if th.Comments[0].AuthorType != "Bot" {
				human++
				continue
			}
			firstLine, _, _ := strings.Cut(th.Comments[0].Body, "\n")
			if m := severityRe.FindString(firstLine); m != "" {
				sev[m]++
			}
			for _, c := range th.Comments[1:] {
				if strings.HasPrefix(strings.TrimSpace(c.Body), "Declined:") {
					declined++
					break
				}
			}
		}
		fmt.Fprintf(b, "- bot review: push rounds %d, threads CRITICAL %d HIGH %d MEDIUM %d LOW %d, Declined: %d\n",
			pushRounds(snap), sev["CRITICAL"], sev["HIGH"], sev["MEDIUM"], sev["LOW"], declined)
		if human > 0 {
			fmt.Fprintf(b, "- human review: threads %d\n", human)
		}
		if ch, ok := criticalHigh(findings); ok && ch == 0 && sev["CRITICAL"]+sev["HIGH"] > 0 {
			signals = append(signals, fmt.Sprintf("escaped: internal review %s, bot review CRITICAL %d HIGH %d", findings, sev["CRITICAL"], sev["HIGH"]))
		}
	}

	for _, s := range signals {
		b.WriteString("- " + s + "\n")
	}
}

func feedbackItem(in FeedbackInput, id string) (Item, bool) {
	for _, it := range in.Items {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}

// runFromEvents is the run outside the day's items as read from the log: kind, est and, when both
// its start and end rows exist, the wall minutes between them (an ended run counts as done).
func runFromEvents(events []Event, id string) Item {
	it := Item{ID: id, Kind: "-", Est: "-"}
	var start, end time.Time
	for _, e := range events {
		if e.ID != id {
			continue
		}
		switch e.Type {
		case "start":
			it.Kind, it.Est, start = e.F[0], e.F[2], e.At
		case "est":
			it.Est = e.F[0]
		case "end":
			end = e.At
		}
	}
	if !start.IsZero() && !end.IsZero() && end.After(start) {
		it.Status = "done"
		it.WallMin = wallMin(start, end)
		it.StartedAt, it.EndedAt = start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339)
	}
	return it
}

// lastShip is the fields of id's last ship row (outcome, step, findings, rounds), nil when none.
func lastShip(events []Event, id string) []string {
	var f []string
	for _, e := range events {
		if e.Type == "ship" && e.ID == id {
			f = e.F
		}
	}
	return f
}

// criticalHigh sums C and H of a "C,H,M,L" findings field; "-" is not ok.
func criticalHigh(findings string) (int, bool) {
	p := strings.Split(findings, ",")
	if len(p) != 4 {
		return 0, false
	}
	c, errC := strconv.Atoi(p[0])
	h, errH := strconv.Atoi(p[1])
	if errC != nil || errH != nil {
		return 0, false
	}
	return c + h, true
}

// pushRounds counts the commits made after the PR was opened.
func pushRounds(s *ReviewSnapshot) int {
	created, err := time.Parse(time.RFC3339, s.CreatedAt)
	if err != nil {
		return 0
	}
	n := 0
	for _, c := range s.Commits {
		if t, err := time.Parse(time.RFC3339, c.CommittedDate); err == nil && t.After(created) {
			n++
		}
	}
	return n
}
