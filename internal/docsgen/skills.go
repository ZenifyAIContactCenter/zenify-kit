package docsgen

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// GenSkills renders skills/<name>.md (znf + coding), agents/<name>.md and
// both indexes. Only frontmatter is read (FR-2.2).
func GenSkills(znf, coding fs.FS) (Files, error) {
	files := Files{}
	var skillRows, agentRows []string

	addSkill := func(fm Frontmatter, ns string) {
		call := "/" + ns + fm.Name
		if fm.ArgumentHint != "" {
			call += " " + fm.ArgumentHint
		}
		var b strings.Builder
		fmt.Fprintf(&b, "---\ntitle: /%s%s\n---\n\n# `%s`\n\n", ns, fm.Name, call)
		fmt.Fprintf(&b, "%s\n\n", escapeAngle(fm.Description))
		fmt.Fprintf(&b, "## Cách gọi\n\n```text\n%s\n```\n\n", call) //znf:allow-lang
		if fm.DisableModelInvocation {
			b.WriteString("::: warning Chỉ user gọi\nSkill này đặt `disable-model-invocation: true`: agent không tự chạy, bạn phải gõ lệnh.\n:::\n\n") //znf:allow-lang
		}
		if fm.AllowedTools != "" {
			fmt.Fprintf(&b, "## Tool được phép\n\n`%s`\n\n", fm.AllowedTools) //znf:allow-lang
		}
		b.WriteString("## Nguồn\n\nSinh bởi `zenify docs gen` từ frontmatter `SKILL.md` trong binary; phần thân skill là agent-read, không hiển thị ở đây.\n") //znf:allow-lang
		files["skills/"+fm.Name+".md"] = []byte(b.String())
		skillRows = append(skillRows, fmt.Sprintf("| [/%s%s](./%s) | %s |", ns, fm.Name, fm.Name, firstSentence(fm.Description)))
	}

	if err := eachSkill(znf, "skills", func(fm Frontmatter) { addSkill(fm, "znf:") }); err != nil {
		return nil, err
	}
	if err := eachSkill(coding, ".", func(fm Frontmatter) { addSkill(fm, "") }); err != nil {
		return nil, err
	}
	agents, _ := fs.ReadDir(znf, "agents")
	for _, e := range agents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, err := fs.ReadFile(znf, "agents/"+e.Name())
		if err != nil {
			return nil, err
		}
		fm, err := ParseFrontmatter(b)
		if err != nil {
			return nil, fmt.Errorf("agents/%s: %w", e.Name(), err)
		}
		var p strings.Builder
		fmt.Fprintf(&p, "---\ntitle: agent %s\n---\n\n# Agent `%s`\n\n%s\n\n", fm.Name, fm.Name, escapeAngle(fm.Description)) //znf:allow-lang
		if fm.Model != "" {
			fmt.Fprintf(&p, "**Model:** `%s`\n\n", fm.Model)
		}
		p.WriteString("## Nguồn\n\nSinh bởi `zenify docs gen` từ frontmatter agent trong binary.\n") //znf:allow-lang
		files["agents/"+fm.Name+".md"] = []byte(p.String())
		agentRows = append(agentRows, fmt.Sprintf("| [%s](./%s) | %s |", fm.Name, fm.Name, firstSentence(fm.Description)))
	}
	sort.Strings(skillRows)
	sort.Strings(agentRows)
	files["skills/index.md"] = index("Skill", "Skill `znf:*` gọi bằng `/znf:<name>`; coding skill gọi bằng `/<name>` sau `zenify skills install`.", skillRows) //znf:allow-lang
	files["agents/index.md"] = index("Agent", "Agent được skill dispatch qua Agent tool; bạn không gọi trực tiếp.", agentRows)                                 //znf:allow-lang
	return files, nil
}

// eachSkill visits <dir>/<name>/SKILL.md; names starting with "_" are shared references, not skills.
func eachSkill(fsys fs.FS, dir string, fn func(Frontmatter)) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil // no such tree → nothing to render
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		p := e.Name() + "/SKILL.md"
		if dir != "." {
			p = dir + "/" + p
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			continue
		}
		fm, err := ParseFrontmatter(b)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		fn(fm)
	}
	return nil
}

func index(title, intro string, rows []string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntitle: %s\n---\n\n# %s\n\n%s\n\n| Tên | Mô tả |\n|---|---|\n", title, title, intro) //znf:allow-lang
	for _, r := range rows {
		b.WriteString(r + "\n")
	}
	return []byte(b.String())
}

func firstSentence(s string) string {
	s = escapeCell(s)
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}
