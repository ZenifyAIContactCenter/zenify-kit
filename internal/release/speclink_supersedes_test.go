package release

import "testing"

func TestParseSpecBrief_Supersedes(t *testing.T) {
	content := []byte("# Design\n\n## Brief\n\n_Supersedes: old-linked-fields_\n_Blast-radius: zenify-kit only_\n")
	got := ParseSpecBrief("specs/zenify-kit/2026-09-09-new-fields-design.md", content)
	if got.Supersedes != "old-linked-fields" {
		t.Fatalf("Supersedes = %q, want %q", got.Supersedes, "old-linked-fields")
	}
}

func TestParseSpecBrief_NoSupersedes(t *testing.T) {
	got := ParseSpecBrief("specs/zenify-kit/2026-09-09-x-design.md", []byte("## Brief\n_DB: N/A_\n"))
	if got.Supersedes != "" {
		t.Fatalf("Supersedes = %q, want empty", got.Supersedes)
	}
}
