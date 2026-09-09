package release

import "testing"

func TestClassifyType(t *testing.T) {
	cases := map[string]string{
		"feat(report): add x":   "feat",
		"fix: y":                "fix",
		"perf(contact): z":      "perf",
		"refactor(report): r":   "refactor",
		"chore: bump":           "chore",
		"Merge pull request #1": "other",
		"random subject":        "other",
	}
	for in, want := range cases {
		if got := ClassifyType(in); got != want {
			t.Errorf("ClassifyType(%q)=%q want %q", in, got, want)
		}
	}
}

func TestParseMergeBranch(t *testing.T) {
	cases := map[string]string{
		"Merge pull request #2022 from ZenifyAIContactCenter/hungnk/hotfix/search/contact": "hungnk/hotfix/search/contact",
		"Merge branch 'release84' of github.com:org/x into release84":                      "release84",
		"feat: not a merge": "",
	}
	for in, want := range cases {
		if got := ParseMergeBranch(in); got != want {
			t.Errorf("ParseMergeBranch(%q)=%q want %q", in, got, want)
		}
	}
}

func TestIsHotfixBranch(t *testing.T) {
	if !IsHotfixBranch("hungnk/hotfix/search/contact") {
		t.Error("hotfix branch should match")
	}
	if !IsHotfixBranch("sonvt/chore/cherry-pick/release84-x") {
		t.Error("cherry-pick branch should match")
	}
	if IsHotfixBranch("feat/improve-time-response") {
		t.Error("feature branch should not match")
	}
}

func TestPathMatchers(t *testing.T) {
	if !IsMigrationPath("db/migrations/2026_add.js") {
		t.Error("migration path")
	}
	if IsMigrationPath("src/app.js") {
		t.Error("non-migration path")
	}
	if !IsTestPath("foo.test.js") || !IsTestPath("bar_test.go") {
		t.Error("test paths")
	}
	if p, ok := MatchesAny("app/models/chat_message.js", []string{"**/chat_*", "**/users*"}); !ok || p == "" {
		t.Error("MatchesAny should hit")
	}
}

func TestParseScope(t *testing.T) {
	cases := map[string]string{
		"feat(linked-fields): add x": "linked-fields",
		"fix(report): tz":            "report",
		"chore: bump":                "",
		"feat: no scope":             "",
		"random subject":             "",
	}
	for in, want := range cases {
		if got := ParseScope(in); got != want {
			t.Errorf("ParseScope(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNormalizeKeyMergesBranchAndScope(t *testing.T) {
	// A branch's last-segment must == scope so they group together.
	if NormalizeKey("namph/feat/linked-fields") != "linked-fields" {
		t.Errorf("branch last-segment: %q", NormalizeKey("namph/feat/linked-fields"))
	}
	if NormalizeKey("linked-fields") != "linked-fields" {
		t.Errorf("scope passthrough")
	}
	if NormalizeKey("Feature/Linked_Fields") != "linked-fields" {
		t.Errorf("case+underscore normalize: %q", NormalizeKey("Feature/Linked_Fields"))
	}
}

func TestHumanizeTitle(t *testing.T) {
	if HumanizeTitle("linked-fields") != "Linked fields" {
		t.Errorf("got %q", HumanizeTitle("linked-fields"))
	}
}

func TestParsePRNum(t *testing.T) {
	if ParsePRNum("Merge pull request #2001 from o/x/y") != "2001" {
		t.Errorf("pr parse")
	}
	if ParsePRNum("feat: no pr") != "" {
		t.Errorf("no pr expected empty")
	}
}
