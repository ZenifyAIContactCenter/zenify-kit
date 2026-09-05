// Package distribute phân phối file config workspace-level từ zenify-knowledge/config/
// xuống workspace theo một manifest. Nguồn = config/, một chiều. Fail-open.
package distribute

import (
	"bytes"
	"path/filepath"
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
	Reason       string // lý do khi SKIP; "" nếu không
}

// escapesRoot báo path rỗng, tuyệt đối, hoặc thoát khỏi thư mục gốc bằng "..".
func escapesRoot(p string) bool {
	if p == "" || filepath.IsAbs(p) {
		return true
	}
	c := filepath.Clean(p)
	return c == ".." || strings.HasPrefix(c, ".."+string(filepath.Separator))
}

// ParseManifest đọc các dòng "<source> <dest>". Bỏ dòng #-đầu và dòng trắng.
// Dòng không đúng đúng 2 trường (thiếu dest, hoặc dư trường) → thêm note, bỏ qua (không lỗi).
func ParseManifest(b []byte) ([]Pair, []string) {
	var pairs []Pair
	var notes []string
	for _, ln := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(ln)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		f := strings.Fields(s)
		if len(f) != 2 {
			notes = append(notes, "bỏ dòng manifest không đúng 2 trường: "+s)
			continue
		}
		pairs = append(pairs, Pair{Source: f[0], Dest: f[1]})
	}
	return pairs, notes
}

// Plan phân loại mỗi Pair bằng cách so nội dung nguồn vs đích. Thuần: mọi I/O
// được inject để test. config/ là nguồn → diff hướng dest→source.
func Plan(pairs []Pair, readSource, readDest func(string) ([]byte, error), isNotFound func(error) bool) []FilePlan {
	out := make([]FilePlan, 0, len(pairs))
	for _, p := range pairs {
		fp := FilePlan{Source: p.Source, Dest: p.Dest}
		if escapesRoot(p.Source) || escapesRoot(p.Dest) {
			fp.State = Skip
			fp.Reason = "path thoát khỏi gốc (tuyệt đối hoặc ..)"
			out = append(out, fp)
			continue
		}
		srcB, err := readSource(p.Source)
		if err != nil {
			fp.State = Skip
			fp.Reason = "không đọc được nguồn"
			out = append(out, fp)
			continue
		}
		dstB, err := readDest(p.Dest)
		if err != nil {
			if isNotFound(err) {
				fp.State = Create
			} else {
				fp.State = Skip
				fp.Reason = "không đọc được đích"
			}
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
