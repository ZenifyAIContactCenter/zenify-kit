package release

import (
	"fmt"
	"sort"
	"strings"
)

// Render dựng markdown report dạng decision-artifact (go/no-go). verbose=true liệt kê
// chore + commit chi tiết; mặc định gập chore. Không trả lỗi/không panic (fail-open):
// người gọi luôn nhận được chuỗi, kể cả report rỗng.
func Render(rep Report, verbose bool) string {
	var b strings.Builder

	// Header.
	fmt.Fprintf(&b, "# Release %d\n", rep.N)
	fmt.Fprintf(&b, "Sinh %s\n\n", rep.GeneratedAt)

	// Headline — quyết định nhanh.
	b.WriteString("## Quyết định nhanh\n")
	if len(rep.ShippingRepos) > 0 {
		fmt.Fprintf(&b, "- Ship: %s\n", strings.Join(rep.ShippingRepos, ", "))
	}
	if len(rep.NotShipped) > 0 {
		fmt.Fprintf(&b, "- Không ship: %s\n", strings.Join(rep.NotShipped, ", "))
	}
	fmt.Fprintf(&b, "- Tổng: %d feature · %d fix · %d hotfix\n", rep.TotalFeat, rep.TotalFix, rep.TotalHotfix)
	// text/template range trên map tự sort key; ở đây sort thủ công để output ổn định.
	for _, p := range sortedKeys(rep.SharedCrossRepo) {
		fmt.Fprintf(&b, "- Shared-contract %s (%s)\n", p, strings.Join(rep.SharedCrossRepo[p], ", "))
	}
	if rep.DeployOrderNote {
		b.WriteString("- Thứ tự deploy: migration → BE → subscriber → FE\n")
	}
	if rep.HotfixesNotSynced > 0 {
		fmt.Fprintf(&b, "- ⚠ %d hotfix chưa sync staging\n", rep.HotfixesNotSynced)
	}
	if len(rep.Migrations) > 0 {
		fmt.Fprintf(&b, "- ⚠ migration: %s\n", strings.Join(rep.Migrations, ", "))
	}
	fmt.Fprintf(&b, "- ℹ %d/%d thay đổi có spec\n", rep.SpecLinked, rep.SpecTotal)

	// Per-repo sections.
	for _, rr := range rep.Repos {
		b.WriteString("\n")
		if rr.Err != "" {
			fmt.Fprintf(&b, "## %s\n", rr.Name)
			fmt.Fprintf(&b, "- ⚠ %s\n", rr.Err)
			continue
		}
		fmt.Fprintf(&b, "## %s (rel%d..%d, cắt %s)\n", rr.Name, rr.PrevRelease, rep.N, cutOr(rr.CutDate))
		fmt.Fprintf(&b, "- Migration %s · Test %s · %d commit → %d thay đổi\n",
			yesNo(rr.HasMigration, "CÓ", "không"),
			yesNo(rr.HasTestTouch, "đụng", "không"),
			len(rr.Commits), len(rr.Changes))
		if len(rr.SharedHits) > 0 {
			fmt.Fprintf(&b, "- ⚠ Shared-collection: %s\n", strings.Join(rr.SharedHits, ", "))
		}
		if rr.RegressionUncomputed {
			b.WriteString("- ⚠ Regression: không so được staging\n")
		}

		feats, fixes, hotfixes, chores := bucketChanges(rr.Changes)
		renderChangeSection(&b, "### Features", feats, verbose)
		renderChangeSection(&b, "### Fixes", fixes, verbose)
		renderChangeSection(&b, "### Hotfixes", hotfixes, verbose)
		// Chore không ảnh hưởng quyết định ship → chỉ hiện khi --verbose (count đã ngầm ở
		// dòng "N commit → M thay đổi"). Mặc định bỏ hẳn để report gọn.
		if verbose {
			renderChangeSection(&b, "### Chores", chores, verbose)
		}
	}

	return b.String()
}

// bucketChanges chia changes theo type: feat→Features, fix→Fixes, hotfix→Hotfixes,
// còn lại (chore/other)→Chores.
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
	// Bảng: mỗi thay đổi một dòng — quét nhanh hơn danh sách dàn trải.
	b.WriteString("| Thay đổi | # | Dev | Spec / Risk | Staging |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, ch := range changes {
		fmt.Fprintf(b, "| %s | %d | %s | %s | %s |\n",
			cell(changeCol(ch)),
			len(ch.Commits),
			cell(devCol(ch.Authors)),
			cell(humanRisk(ch.Risk)),
			cell(stagingCol(ch.NotOnStaging)))
	}
	// FR-5.2: verbose liệt kê commit của từng thay đổi bên dưới bảng (bảng không lồng được).
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

// changeCol dựng ô "Thay đổi": **Title**[ #PR] — Desc (bỏ "— Desc" khi Desc rỗng).
func changeCol(ch Change) string {
	s := "**" + ch.Title + "**" + prNum(ch.PRNum)
	if ch.Desc != "" {
		s += " — " + ch.Desc
	}
	return s
}

// devCol join Authors; rỗng → "—".
func devCol(authors []string) string {
	if len(authors) == 0 {
		return "—"
	}
	return strings.Join(authors, ", ")
}

// stagingCol: NotOnStaging → cảnh báo chưa sync; else ok.
func stagingCol(notOnStaging bool) string {
	if notOnStaging {
		return "⚠ chưa sync"
	}
	return "ok"
}

// cell escape ký tự phá bảng markdown: `|` và xuống dòng.
func cell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}

// humanRisk render risk gọn cho ô bảng. SpecPath rỗng = "unknown — no spec".
func humanRisk(r RiskMeta) string {
	if r.SpecPath == "" {
		return "unknown — no spec"
	}
	return fmt.Sprintf("Blast: %s · DB: %s · Rollback: %s", r.BlastRadius, r.DB, r.Rollback)
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
		return "chưa xác định"
	}
	return cut
}

// sortedKeys trả key của map đã sort (thay cho text/template auto-sort cũ).
func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
