package speclife

import (
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/release"
)

func TestBuildContracts_SkipsMissingTags(t *testing.T) {
	specs := []release.SpecMeta{
		{Path: "specs/contact-center-be/a-design.md", BlastRadius: "contact-center-be, chatting", DB: "chat_rooms"},
		{Path: "specs/zenify-kit/b-design.md"}, // no tags → skipped
	}
	cs, skipped := BuildContracts(specs)
	if len(cs) != 1 {
		t.Fatalf("len(contracts) = %d, want 1", len(cs))
	}
	if skipped != 1 {
		t.Fatalf("skipped = %d, want 1", skipped)
	}
	if cs[0].Repo != "contact-center-be" {
		t.Fatalf("Repo = %q", cs[0].Repo)
	}
}

func TestFilterByCollection(t *testing.T) {
	cs := []Contract{
		{SpecPath: "a", DB: "chat_rooms, users"},
		{SpecPath: "b", DB: "tickets"},
	}
	got := FilterByCollection(cs, "users")
	if len(got) != 1 || got[0].SpecPath != "a" {
		t.Fatalf("got %+v, want only a", got)
	}
}

func TestFilterByRepo(t *testing.T) {
	cs := []Contract{
		{SpecPath: "a", BlastRadius: "contact-center-be, chatting"},
		{SpecPath: "b", BlastRadius: "zenify-kit only"},
	}
	got := FilterByRepo(cs, "chatting")
	if len(got) != 1 || got[0].SpecPath != "a" {
		t.Fatalf("got %+v, want only a", got)
	}
	// substring that is not a whole token must NOT match ("kit" inside "zenify-kit" is a token via '-')
	if n := len(FilterByRepo(cs, "zenify-kit")); n != 1 {
		t.Fatalf("zenify-kit match count = %d, want 1", n)
	}
}
