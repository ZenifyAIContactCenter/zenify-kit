package release

import (
	"fmt"
	"sort"
	"strings"
)

// Render builds a decision-artifact-style markdown report (go/no-go). verbose=true lists chores
// + detailed commits; by default chores are collapsed. It never returns an error/never panics
// (fail-open): the caller always gets a string back, even for an empty report.
func Render(rep Report, verbose bool) string {
	var b strings.Builder

	// Header.
	if rep.Unreleased {
		// The unreleased view regenerates on every /ship + docs-sync → do NOT print a
		// time.Now() timestamp (that would produce a no-op churn commit that only changes the
		// time, in the store). The moment is already in the store's git-log; this keeps output
		// deterministic against git-state (SC-6). A once-cut R<N>.md still keeps its "Sinh" line.
		fmt.Fprintf(&b, "# Release đang hình thành: R%d (chưa deploy)\n\n", rep.N) //znf:allow-lang
	} else {
		fmt.Fprintf(&b, "# Release %d\n", rep.N)
		fmt.Fprintf(&b, "Sinh %s\n\n", rep.GeneratedAt)
	}

	// Headline — quick decision.
	b.WriteString("## Quyết định nhanh\n") //znf:allow-lang
	if len(rep.ShippingRepos) > 0 {
		fmt.Fprintf(&b, "- Ship: %s\n", strings.Join(rep.ShippingRepos, ", "))
	}
	if len(rep.NotShipped) > 0 {
		fmt.Fprintf(&b, "- Không ship: %s\n", strings.Join(rep.NotShipped, ", ")) //znf:allow-lang
	}
	fmt.Fprintf(&b, "- Tổng: %d feature · %d fix · %d hotfix\n", rep.TotalFeat, rep.TotalFix, rep.TotalHotfix) //znf:allow-lang
	// text/template's map range auto-sorts keys; here we sort manually for stable output.
	for _, p := range sortedKeys(rep.SharedCrossRepo) {
		fmt.Fprintf(&b, "- Shared-contract %s (%s)\n", p, strings.Join(rep.SharedCrossRepo[p], ", "))
	}
	if rep.DeployOrderNote {
		b.WriteString("- Thứ tự deploy: migration → BE → subscriber → FE\n") //znf:allow-lang
	}
	if rep.HotfixesNotSynced > 0 {
		fmt.Fprintf(&b, "- ⚠ %d hotfix chưa sync staging\n", rep.HotfixesNotSynced) //znf:allow-lang
	}
	if len(rep.Migrations) > 0 {
		fmt.Fprintf(&b, "- ⚠ migration: %s\n", strings.Join(rep.Migrations, ", "))
	}
	fmt.Fprintf(&b, "- ℹ %d/%d thay đổi có spec\n", rep.SpecLinked, rep.SpecTotal) //znf:allow-lang

	// Per-repo sections.
	for _, rr := range rep.Repos {
		b.WriteString("\n")
		if rr.Err != "" {
			fmt.Fprintf(&b, "## %s\n", rr.Name)
			fmt.Fprintf(&b, "- ⚠ %s\n", rr.Err)
			continue
		}
		if rep.Unreleased {
			fmt.Fprintf(&b, "## %s (rel%d..staging)\n", rr.Name, rr.PrevRelease)
		} else {
			fmt.Fprintf(&b, "## %s (rel%d..%d, cắt %s)\n", rr.Name, rr.PrevRelease, rep.N, cutOr(rr.CutDate)) //znf:allow-lang
		}
		fmt.Fprintf(&b, "- Migration %s · Test %s · %d commit → %d thay đổi\n", //znf:allow-lang
			yesNo(rr.HasMigration, "CÓ", "không"),   //znf:allow-lang
			yesNo(rr.HasTestTouch, "đụng", "không"), //znf:allow-lang
			len(rr.Commits), len(rr.Changes))
		if len(rr.SharedHits) > 0 {
			fmt.Fprintf(&b, "- ⚠ Shared-collection: %s\n", strings.Join(rr.SharedHits, ", "))
		}
		if rr.RegressionUncomputed {
			b.WriteString("- ⚠ Regression: không so được staging\n") //znf:allow-lang
		}

		feats, fixes, hotfixes, chores := bucketChanges(rr.Changes)
		renderChangeSection(&b, "### Features", feats, verbose)
		renderChangeSection(&b, "### Fixes", fixes, verbose)
		renderChangeSection(&b, "### Hotfixes", hotfixes, verbose)
		// Risk metadata (Blast/DB/Rollback) lives below the table as **Label:** value lines —
		// multi-sentence prose doesn't fit in a table cell. Only a change WITH a spec gets this block.
		renderRiskDetail(&b, rr.Changes)
		// A chore does not affect the ship decision → only shown with --verbose (its count is
		// already implicit in the "N commit → M change" line). Dropped entirely by default to keep the report terse.
		if verbose {
			renderChangeSection(&b, "### Chores", chores, verbose)
		}
	}

	return b.String()
}

// bucketChanges splits changes by type: feat→Features, fix→Fixes, hotfix→Hotfixes,
// everything else (chore/other)→Chores.
func bucketChanges(changes []Change) (feats, fixes, hotfixes, chores []Change) {
	for _, ch := range changes {
		switch ch.Type {
		case "feat":
			feats = append(feats, ch)
		case "fix":
			fixes = append(fixes, ch)
		case "hotfix":
			hotfixes = append(hotfixes, ch)
		default:
			chores = append(chores, ch)
		}
	}
	return
}

func renderChangeSection(b *strings.Builder, header string, changes []Change, verbose bool) {
	if len(changes) == 0 {
		return
	}
	b.WriteString(header + "\n")
	// Table = the quick-scan layer: every row is short and even. The "Mô tả" column is a one-line //znf:allow-lang
	// prose (usually Vietnamese) taken from the _Release-Note trailer written by /ship — so
	// reading the doc tells you what the PR does without opening it. The Spec column is just a
	// ✓/— flag (risk detail lives in the "#### Rủi ro" block below the table). //znf:allow-lang
	b.WriteString("| Thay đổi | Mô tả | # | Dev | Spec | Staging |\n") //znf:allow-lang
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, ch := range changes {
		fmt.Fprintf(b, "| %s | %s | %d | %s | %s | %s |\n",
			cell(changeCol(ch)),
			cell(descCol(ch)),
			len(ch.Commits),
			cell(devCol(ch.Authors)),
			specCol(ch.Risk),
			cell(stagingCol(ch.NotOnStaging)))
	}
	// FR-5.2: verbose lists each change's commits below the table (a table can't nest one).
	if verbose {
		for _, ch := range changes {
			var subs []string
			for _, c := range ch.Commits {
				if c.Subject != "" {
					subs = append(subs, c.Subject)
				}
			}
			if len(subs) == 0 {
				continue
			}
			fmt.Fprintf(b, "\n**%s**%s\n", ch.Title, prNum(ch.PRNum))
			for _, s := range subs {
				fmt.Fprintf(b, "- %s\n", s)
			}
		}
	}
}

// changeCol builds the "Thay đổi" cell: **Title**[ #PR] — a short label for scanning. Desc (the //znf:allow-lang
// commit subject, often a mix of English/Vietnamese and long) is NO LONGER pasted in here; it only shows with --verbose.
func changeCol(ch Change) string {
	return "**" + ch.Title + "**" + prNum(ch.PRNum)
}

// descCol builds the "Mô tả" cell: a one-line description from _Release-Note (Risk.Note). Empty //znf:allow-lang
// (the change has no note-commit, or its risk came from a spec) → "—". It does NOT fall back to
// Change.Desc (the commit subject, often a mix of English/Vietnamese) — that is exactly what this
// table is trying to avoid; a blank cell is a nudge for /ship to write --note.
func descCol(ch Change) string {
	if ch.Risk.Note == "" {
		return "—"
	}
	return ch.Risk.Note
}

// devCol joins Authors; empty → "—".
func devCol(authors []string) string {
	if len(authors) == 0 {
		return "—"
	}
	return strings.Join(authors, ", ")
}

// stagingCol: NotOnStaging → a not-synced warning; else ok.
func stagingCol(notOnStaging bool) string {
	if notOnStaging {
		return "⚠ chưa sync" //znf:allow-lang
	}
	return "ok"
}

// cell escapes characters that would break a markdown table: `|` and newlines.
func cell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}

// specCol: a compact flag for the Spec column — ✓ if linked to a spec, — if not.
func specCol(r RiskMeta) string {
	if r.SpecPath == "" {
		return "—"
	}
	return "✓"
}

// renderRiskDetail prints the "#### Rủi ro" block below the table for changes WITH a spec, each //znf:allow-lang
// tag as one **Label:** value line (matching artifact-style). No change has a spec → skip entirely.
func renderRiskDetail(b *strings.Builder, changes []Change) {
	var spec []Change
	for _, ch := range changes {
		// Excluding chore/other MIRRORS the headline SpecTotal (release.go:175): their table row
		// is verbose-gated under "### Chores", so a non-verbose risk block would otherwise
		// reference a change that doesn't appear in any table above it.
		if ch.Type == "chore" || ch.Type == "other" {
			continue
		}
		if ch.Risk.SpecPath != "" {
			spec = append(spec, ch)
		}
	}
	if len(spec) == 0 {
		return
	}
	b.WriteString("\n#### Rủi ro (thay đổi có spec)\n") //znf:allow-lang
	for _, ch := range spec {
		fmt.Fprintf(b, "\n**%s**\n", riskHeader(ch))
		fmt.Fprintf(b, "- **Blast-radius:** %s\n", oneLine(ch.Risk.BlastRadius))
		fmt.Fprintf(b, "- **DB:** %s\n", oneLine(ch.Risk.DB))
		fmt.Fprintf(b, "- **Rollback:** %s\n", oneLine(ch.Risk.Rollback))
	}
}

// riskHeader: "#PR — Title" when there's a PR, else just Title.
func riskHeader(ch Change) string {
	if ch.PRNum != "" {
		return "#" + ch.PRNum + " — " + ch.Title
	}
	return ch.Title
}

// oneLine collapses newlines into spaces so the value fits neatly on one bullet line; "" → "—".
// It also trims trailing emphasis/backtick markers: the speclink regex captures the "**_Label:**"
// tag only up to "_Label:", so the label's CLOSING "**" leaks into the start of the value
// ("** contact…"); trimming here keeps the display clean.
func oneLine(s string) string {
	s = strings.Trim(strings.ReplaceAll(s, "\n", " "), "`* ")
	if s == "" {
		return "—"
	}
	return s
}

func prNum(pr string) string {
	if pr == "" {
		return ""
	}
	return " #" + pr
}

func yesNo(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

func cutOr(cut string) string {
	if cut == "" {
		return "chưa xác định" //znf:allow-lang
	}
	return cut
}

// sortedKeys returns the map's keys sorted (replacing text/template's old auto-sort).
func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
