package release

import (
	"strings"
	"text/template"
)

// text/template range trên map tự sort key → output ổn định cho TypeCounts/SharedCrossRepo.
var tmpl = template.Must(template.New("r").Funcs(template.FuncMap{
	"join": strings.Join,
}).Parse(`# Release {{.N}} — risk report
Sinh lúc: {{.GeneratedAt}}
{{if .NotShipped}}Không ship tuần này: {{join .NotShipped ", "}}{{end}}

## Tổng quan
{{if .DeployOrderNote}}- Thứ tự deploy nếu lên đồng loạt: migration → BE → subscriber → FE
{{end}}{{range $p, $rs := .SharedCrossRepo}}- Shared-collection ≥2 repo: {{$p}} ({{join $rs ", "}})
{{end}}
{{range .Repos}}## {{.Name}}   (release{{.PrevRelease}}..release{{$.N}})
{{if .Err}}- ⚠ {{.Err}}
{{else}}- Ngày cắt: {{if .CutDate}}{{.CutDate}}{{else}}chưa xác định{{end}}
- Ship: {{len .Commits}} commit{{range $t, $c := .TypeCounts}} · {{$t}} {{$c}}{{end}}
- Migration: {{if .HasMigration}}CÓ{{else}}không{{end}}    Test: {{if .HasTestTouch}}có đụng{{else}}không{{end}}
{{if .SharedHits}}- Shared-collection: {{join .SharedHits ", "}}
{{end}}{{if .RegressionUncomputed}}- ⭐ Regression: chưa tính được (không so sánh được với origin/staging)
{{else if .Regression}}- ⭐ Regression — {{len .Regression}} commit chưa có trên staging (kiểm tra cherry-pick):
{{range .Regression}}    {{.SHA}} {{.Subject}}
{{end}}{{end}}{{if .Hotfixes}}- Hotfix trên release: {{len .Hotfixes}}
{{range .Hotfixes}}    {{.SHA}} {{.Subject}}
{{end}}{{end}}{{end}}
{{end}}`))

// Render dựng markdown report. Không trả lỗi (fail-open): lỗi template → chuỗi rỗng/dở, người gọi vẫn tiếp.
func Render(rep Report) string {
	var b strings.Builder
	_ = tmpl.Execute(&b, rep)
	return b.String()
}
