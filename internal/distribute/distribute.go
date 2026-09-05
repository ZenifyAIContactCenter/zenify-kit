// Package distribute phân phối file config workspace-level từ zenify-knowledge/config/
// xuống workspace theo một manifest. Nguồn = config/, một chiều. Fail-open.
package distribute

import (
	"bytes"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

type State string

const (
	Create State = "CREATE"
	Same   State = "SAME"
	Update State = "UPDATE"
	Skip   State = "SKIP"
)

// Pair là một dòng manifest: file nguồn (trong config dir) → đích (trong workspace).
type Pair struct{ Source, Dest string }

// FilePlan là phân loại cho một Pair sau khi so nội dung.
type FilePlan struct {
	Source, Dest string
	State        State
	Diff         string // unified diff cho UPDATE; "" nếu không
}

// ParseManifest đọc các dòng "<source> <dest>". Bỏ dòng #-đầu và dòng trắng.
// Dòng chỉ có một trường (thiếu dest) → thêm note, bỏ qua (không lỗi).
func ParseManifest(b []byte) ([]Pair, []string) {
	var pairs []Pair
	var notes []string
	for _, ln := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(ln)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		f := strings.Fields(s)
		if len(f) < 2 {
			notes = append(notes, "bỏ dòng manifest thiếu đích: "+s)
			continue
		}
		pairs = append(pairs, Pair{Source: f[0], Dest: f[1]})
	}
	return pairs, notes
}

// Plan phân loại mỗi Pair bằng cách so nội dung nguồn vs đích. Thuần: mọi I/O
// được inject để test. config/ là nguồn → diff hướng dest→source.
func Plan(pairs []Pair, readSource, readDest func(string) ([]byte, error)) []FilePlan {
	out := make([]FilePlan, 0, len(pairs))
	for _, p := range pairs {
		fp := FilePlan{Source: p.Source, Dest: p.Dest}
		srcB, err := readSource(p.Source)
		if err != nil {
			fp.State = Skip
			out = append(out, fp)
			continue
		}
		dstB, err := readDest(p.Dest)
		if err != nil {
			fp.State = Create
			out = append(out, fp)
			continue
		}
		if bytes.Equal(srcB, dstB) {
			fp.State = Same
			out = append(out, fp)
			continue
		}
		fp.State = Update
		fp.Diff = unifiedDiff(dstB, srcB, p.Dest, p.Source)
		out = append(out, fp)
	}
	return out
}

func unifiedDiff(dst, src []byte, fromName, toName string) string {
	text, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(dst)),
		B:        difflib.SplitLines(string(src)),
		FromFile: fromName,
		ToFile:   toName,
		Context:  3,
	})
	return strings.TrimRight(text, "\n")
}

// Apply ghi các file CREATE/UPDATE (đọc lại nguồn qua readSource, ghi qua writeDest).
// SAME/SKIP không ghi. Fail-open: lỗi ghi/đọc thành note, không dừng.
func Apply(plans []FilePlan, readSource func(string) ([]byte, error), writeDest func(dest string, data []byte) error) []string {
	var notes []string
	for _, p := range plans {
		if p.State != Create && p.State != Update {
			continue
		}
		b, err := readSource(p.Source)
		if err != nil {
			notes = append(notes, "không đọc được nguồn "+p.Source+": "+err.Error())
			continue
		}
		if err := writeDest(p.Dest, b); err != nil {
			notes = append(notes, "không ghi được "+p.Dest+": "+err.Error())
			continue
		}
		notes = append(notes, "đã ghi "+p.Dest)
	}
	return notes
}
