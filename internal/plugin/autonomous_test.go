package plugin

import (
	"strings"
	"testing"
)

// FR-5.1, FR-5.2, FR-5.6, FR-5.7: one shared doc overrides every human gate when ZNF_AUTONOMOUS=1.
func TestAutonomousDoc(t *testing.T) {
	s := syncedAsset(t, "skills/_shared/autonomous.md")
	assertContainsAll(t, "autonomous.md", s, []string{
		"ZNF_AUTONOMOUS=1", "AskUserQuestion",
		"zenify await spec", "zenify await plan", "zenify await data-write",
		"zenify await irreversible", "zenify await needs-human",
		"Ruling: <decision> — <why> — <cost if wrong>",
		"resume <id> approved", "resume <id> rejected:", "zenify await show <id>",
		"--summary", "--file", "--command", "never merge",
		"znf:ui-verify-ok: <reason> (await <id>)", "zenify ui-verify check",
	})
	// Precedence: gates a Ruling never replaces, named, and above the map.
	precedence := []string{
		"## Precedence", "never replaces", "a prd write other than a data script", "`prd needs role lead`",
		"deleting data that cannot be recovered, a production deploy, revert or disable, credentials, or a security-sensitive action",
		"`hotfix` step 2", "step 4", "Delete them (unrecoverable)", "commit or move",
		"says STOP", "`fix` Step 0", "`cook` Step 6", "`gh stack`", "`too-large`",
		"`finishing-a-development-branch` Step 3", "zenify wt base",
		"per-section design approval is a `Ruling`", "only through the spec await",
		"credential, token or `.env` content", "only from the host", "no approval state",
	}
	assertContainsAll(t, "autonomous.md", s, precedence)
	// FR-09, FR-10.6: asking goes through `zenify await question`; the plan gate is conditional.
	assertContainsAll(t, "autonomous.md", s, []string{
		"Ask a person only through `zenify await question`, and only when the answer changes behaviour, acceptance, scope, or leads to something hard to reverse. Everything else: decide and write a `Ruling:` line.",
		"Before a spec exists, write each Ruling under the spec's `## Clarifications`; during SDD, in the ledger.",
		"At most 2 question rounds before the spec. Still unclear after round 2: `zenify await needs-human` listing the missing points.",
		"Before every `zenify await question`, write `.znf/dialogue.md` at the workspace root: questions asked, answers received, draft decisions. On resume, read it and never re-ask an answered question.",
		"A `business` question is read by a non-developer: plain domain language, no file names, no code.",
		"A question after the spec is approved: commit work-in-progress, write `task N/M` in the SDD ledger, then await. If the answer changes the spec, edit it and send `zenify await spec` again with a summary starting `Spec changed`.",
		"| cook Step 5b plan approval | zenify await plan only when the plan touches 2+ repos, a file matching .github/risk-paths.json of the target repo (absent file = no match), or znf:analyze reports CRITICAL/HIGH; otherwise a Ruling |",
		"| brainstorming clarifying questions | zenify await question (rules above) |",
		"resume <id> answered: <text>",
		"Keep pre-spec Rulings in `.znf/dialogue.md` and copy them into the spec's `## Clarifications` when you write it; from spec to SDD (e.g. Step 5b), add them to the spec's `## Clarifications` too.",
		"`brainstorming` step 8 (clarify-lite) is a clarifying question round too",
		"inconclusive → `zenify await question`",
	})
	if strings.Contains(s, "Never ask") {
		t.Error("autonomous.md still says \"Never ask\"")
	}
	if p, m := strings.Index(s, "## Precedence"), strings.Index(s, "## Gate map"); p < 0 || m < 0 || p > m {
		t.Errorf("## Precedence must come before ## Gate map")
	}
	// One table row per human gate of spec FR-5.2, plus the ui-verify stop of FR-08.4 and the
	// clarifying-question row of FR-09: eleven in all.
	rows := []string{"`brainstorming` steps 2, 7 (", "brainstorming clarifying questions", "`brainstorming` step 12", "cook Step 5b plan approval", "`fix` Step 4",
		"`ship` fix-loop", "`ship` step 4", "`_shared/data-scripts.md`", "Four things stop you\": first three", "Four things stop you\": plan too broken", "`discipline`"}
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "| ") && !strings.HasPrefix(line, "| Human gate") {
			n++
		}
	}
	if n != len(rows) {
		t.Errorf("autonomous.md table has %d rows, want %d", n, len(rows))
	}
	for _, r := range rows {
		found := false
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(line, "| ") && strings.Contains(line, r) {
				found = true
			}
		}
		if !found {
			t.Errorf("autonomous.md has no table row for %q", r)
		}
	}
	assertProjectAgnostic(t, "autonomous.md", s)
}

// FR-09: clarifying questions are asked (and answered) before the plan gate runs.
func TestAutonomousQuestionGateOrder(t *testing.T) {
	s := syncedAsset(t, "skills/_shared/autonomous.md")
	q, p := strings.Index(s, "| brainstorming clarifying questions |"), strings.Index(s, "| cook Step 5b")
	if q < 0 || p < 0 || q > p {
		t.Errorf("the brainstorming clarifying questions row must come before the cook Step 5b row (at %d, %d)", q, p)
	}
}

// FR-03.1: the worker sandbox now carries contact-center-web, so the run skill must say how to start it.
func TestAutonomousSandboxFE(t *testing.T) {
	s := syncedAsset(t, "skills/run/references/autonomous-sandbox.md")
	assertContainsAll(t, "autonomous-sandbox.md", s, []string{
		"ZNF_SANDBOX_WEB_PORT", "contact-center-web", "zenify e2e run --local",
		"zenify e2e login --port $ZNF_SANDBOX_WEB_PORT --role <role>",
	})
	if strings.Contains(s, "There is no FE in the sandbox") {
		t.Error("autonomous-sandbox.md still claims there is no FE in the sandbox")
	}
}
