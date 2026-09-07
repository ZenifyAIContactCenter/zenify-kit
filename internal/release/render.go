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
	fmt.Fprintf(&b, "# Release %d — go/no-go\n", rep.N)
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
		if len(chores) > 0 {
			if verbose {
				renderChangeSection(&b, "### Chores", chores, verbose)
			} else {
				fmt.Fprintf(&b, "### Chores (ẩn %d — --verbose)\n", len(chores))
			}
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
	for _, ch := range changes {
		fmt.Fprintf(b, "- **%s**%s (%d commit)\n", ch.Title, prNum(ch.PRNum), len(ch.Commits))
		fmt.Fprintf(b, "  %s\n", humanRisk(ch.Risk))
		if ch.NotOnStaging {
			b.WriteString("  ⚠ CHƯA trên staging → cần cherry-pick về staging\n")
		}
		// FR-5.2: verbose in danh sách commit của mỗi thay đổi (bỏ subject rỗng).
		if verbose {
			for _, c := range ch.Commits {
				if c.Subject == "" {
					continue
				}
				fmt.Fprintf(b, "  - %s\n", c.Subject)
			}
		}
	}
}

// humanRisk render dòng risk từ RiskMeta. SpecPath rỗng = "unknown — no spec".
func humanRisk(r RiskMeta) string {
	if r.SpecPath == "" {
		return "Blast: unknown — no spec · DB: — · Rollback: —"
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
