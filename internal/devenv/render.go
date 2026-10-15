package devenv

import (
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

// RenderHeader is the first line of a rendered credential file.
const RenderHeader = "# zenify-kit — credential dev. Kit giữ khung; bạn chỉ điền giá trị. Key trống = chưa cấu hình." //znf:allow-lang

// Section titles of a rendered credential file, in output order.
const (
	sectionPrefix     = "# ══"
	titleStaging      = "# ══ Staging ══"
	titleTooling      = "# ══ Tooling ══"
	titleProduction   = "# ══ Production ══"
	titleCustom       = "# ══ Custom (ngoài manifest) ══" //znf:allow-lang
	labelHowTo        = "Lấy: "                           //znf:allow-lang
	labelSecret       = "secret"
	labelNotSecret    = "không secret" //nolint:gosec // G101 -- a label, not a credential //znf:allow-lang
	commentPartJoiner = " · "
)

// RenderResult is the rendered file plus what changed, by key name only.
type RenderResult struct {
	File        *File
	Added       []string // manifest keys the old file had no assignment for
	Removed     []string // previously managed keys no longer in the manifest, deleted
	Undescribed []string // custom keys without a description line
	Shadowed    []string // keys whose other non-empty assignments were kept as comments
}

// Render rebuilds the credential file as a grouped scaffold: header, then the
// staging, tooling and production sections of manifest keys, then a custom
// section for every other key and every kept line of old. It does no I/O;
// a nil old is an empty file. Values pass through parseLine and formatValue
// unchanged, and rendering a rendered file returns it byte for byte.
func Render(old *File, keys []manifest.PlacedKey, prevManaged []string) RenderResult {
	if old == nil {
		old = &File{}
	}
	managed := map[string]bool{}
	generated := map[string]bool{} // "KEY\x00comment" of every kit comment line
	for _, k := range keys {
		managed[k.Name] = true
		generated[k.Name+"\x00"+descComment(k)] = true
		generated[k.Name+"\x00"+noteComment(k)] = true
	}
	var res RenderResult
	removed := map[string]bool{}
	for _, k := range prevManaged {
		if managed[k] || removed[k] {
			continue
		}
		removed[k] = true
		if _, ok := old.Get(k); ok {
			res.Removed = append(res.Removed, k)
		}
	}

	// Every assignment of a key, in file order. The rendered value is the
	// last non-empty one (empty = unset); every other distinct non-empty
	// value is kept as a commented assignment, so a duplicate never loses one.
	vals := map[string][]string{}
	var assignOrder []string
	for _, l := range old.lines {
		if k, v, ok := parseLine(l); ok {
			if _, dup := vals[k]; !dup {
				assignOrder = append(assignOrder, k)
			}
			vals[k] = append(vals[k], v)
		}
	}
	value := func(k string) string {
		vs := vals[k]
		for i := len(vs) - 1; i >= 0; i-- {
			if vs[i] != "" {
				return vs[i]
			}
		}
		return ""
	}

	// Classify old lines. A comment block directly above a manifest (or
	// removed) key is kit-generated and dropped, except a commented-out
	// assignment the kit did not write (it may hold an old value); above a
	// custom key its last line is the description; every other non-blank
	// line is kept.
	var customOrder, kept, block []string
	desc := map[string]string{}
	seen := map[string]bool{}
	flush := func() {
		kept = append(kept, block...)
		block = nil
	}
	for _, l := range old.lines {
		t := strings.TrimSpace(l)
		if t == "" || t == RenderHeader || strings.HasPrefix(t, sectionPrefix) {
			flush()
			continue
		}
		k, _, ok := parseLine(l)
		if !ok {
			if strings.HasPrefix(t, "#") {
				block = append(block, l)
				continue
			}
			flush()
			kept = append(kept, l)
			continue
		}
		if managed[k] || removed[k] {
			for _, c := range block {
				if _, isDesc := descLine(c); !isDesc && !generated[k+"\x00"+strings.TrimSpace(c)] {
					kept = append(kept, c)
				}
			}
			block = nil
			continue
		}
		if !seen[k] {
			seen[k] = true
			customOrder = append(customOrder, k)
		}
		if n := len(block); n > 0 {
			if d, isDesc := descLine(block[n-1]); isDesc && d != "" {
				desc[k] = d
				block = block[:n-1]
			}
		}
		flush()
	}
	flush()
	for _, k := range assignOrder {
		if removed[k] {
			continue
		}
		v, done := value(k), map[string]bool{}
		for _, sv := range vals[k] {
			if sv == "" || sv == v || done[sv] {
				continue
			}
			if len(done) == 0 {
				res.Shadowed = append(res.Shadowed, k)
			}
			done[sv] = true
			kept = append(kept, "# "+assignLine(k, sv))
		}
	}

	// The first entry of a name wins; a group outside the three folds into
	// staging, so a managed key's value is never consumed without a home.
	groupOf := map[string]string{}
	for _, k := range keys {
		if _, dup := groupOf[k.Name]; dup {
			continue
		}
		switch g := k.EffectiveGroup(); g {
		case manifest.GroupTooling, manifest.GroupProduction:
			groupOf[k.Name] = g
		default:
			groupOf[k.Name] = manifest.GroupStaging
		}
	}
	groupOrder := []string{manifest.GroupStaging, manifest.GroupTooling, manifest.GroupProduction}
	groups := map[string][][]string{}
	for _, g := range groupOrder {
		placed := map[string]bool{}
		for _, k := range keys {
			if groupOf[k.Name] != g || placed[k.Name] {
				continue
			}
			placed[k.Name] = true
			if _, ok := vals[k.Name]; !ok {
				res.Added = append(res.Added, k.Name)
			}
			groups[g] = append(groups[g], []string{
				descComment(k),
				noteComment(k),
				assignLine(k.Name, value(k.Name)),
			})
		}
	}

	var custom [][]string
	for _, k := range customOrder {
		b := []string{assignLine(k, value(k))}
		if d, ok := desc[k]; ok {
			b = append([]string{"# " + d}, b...)
		} else {
			res.Undescribed = append(res.Undescribed, k)
		}
		custom = append(custom, b)
	}
	if len(kept) > 0 {
		custom = append(custom, kept)
	}

	out := []string{RenderHeader}
	section := func(title string, blocks [][]string) {
		if len(blocks) == 0 {
			return
		}
		out = append(out, "", title)
		for i, b := range blocks {
			if i > 0 {
				out = append(out, "")
			}
			out = append(out, b...)
		}
	}
	section(titleStaging, groups[manifest.GroupStaging])
	section(titleTooling, groups[manifest.GroupTooling])
	section(titleProduction, groups[manifest.GroupProduction])
	section(titleCustom, custom)
	res.File = &File{lines: out}
	return res
}

// descComment is the first comment line of a manifest key.
func descComment(k manifest.PlacedKey) string {
	return "# " + oneLine(k.Description)
}

// noteComment is the second comment line of a manifest key.
func noteComment(k manifest.PlacedKey) string {
	var parts []string
	if k.HowTo != "" {
		parts = append(parts, labelHowTo+oneLine(k.HowTo))
	}
	if k.Scope != "" {
		parts = append(parts, oneLine(k.Scope))
	}
	if k.Secret {
		parts = append(parts, labelSecret)
	} else {
		parts = append(parts, labelNotSecret)
	}
	if k.Repo != "" {
		parts = append(parts, oneLine(k.Repo))
	}
	return "# " + strings.Join(parts, commentPartJoiner)
}

func assignLine(key, value string) string {
	if value == "" {
		return key + "="
	}
	return key + "=" + formatValue(value)
}

// oneLine flattens newlines: a second line could otherwise parse as a key.
func oneLine(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}
