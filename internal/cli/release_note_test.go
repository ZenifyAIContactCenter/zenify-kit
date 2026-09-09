package cli

import (
	"strings"
	"testing"
)

func TestBuildNoteMessageTagsAndDefaults(t *testing.T) {
	m := buildNoteMessage("report-table", "add table", "", "", "", "")
	for _, want := range []string{
		"chore(release): note report-table",
		"_Release-Slug: report-table",
		"_Release-Note: add table",
		"_Blast-radius: unknown", // default when empty
		"_DB: N/A",
		"_Rollback: revert PR",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("message missing %q:\n%s", want, m)
		}
	}
	if strings.Contains(m, "Spec:") {
		t.Errorf("without --spec, should NOT print a Spec: line")
	}
}

func TestBuildNoteMessageSpecLine(t *testing.T) {
	m := buildNoteMessage("x", "n", "be+web", "N/A", "revert", "specs/zenify-kit/x-design.md")
	if !strings.Contains(m, "Spec: specs/zenify-kit/x-design.md") {
		t.Errorf("with --spec, must print a Spec: line:\n%s", m)
	}
	if !strings.Contains(m, "_Blast-radius: be+web") {
		t.Errorf("must keep the blast value passed by the caller:\n%s", m)
	}
}
