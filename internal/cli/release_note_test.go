package cli

import (
	"strings"
	"testing"
)

func TestBuildNoteMessageTagsAndDefaults(t *testing.T) {
	m := buildNoteMessage("report-table", "add table", "", "", "", "")
	for _, want := range []string{
		"chore(release): note report-table",
		"_Release-Note: add table",
		"_Blast-radius: unknown", // default khi rỗng
		"_DB: N/A",
		"_Rollback: revert PR",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("message thiếu %q:\n%s", want, m)
		}
	}
	if strings.Contains(m, "Spec:") {
		t.Errorf("không có --spec thì KHÔNG in dòng Spec:")
	}
}

func TestBuildNoteMessageSpecLine(t *testing.T) {
	m := buildNoteMessage("x", "n", "be+web", "N/A", "revert", "specs/zenify-kit/x-design.md")
	if !strings.Contains(m, "Spec: specs/zenify-kit/x-design.md") {
		t.Errorf("có --spec phải in dòng Spec:\n%s", m)
	}
	if !strings.Contains(m, "_Blast-radius: be+web") {
		t.Errorf("giữ blast do người gọi truyền:\n%s", m)
	}
}
