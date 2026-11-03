package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The tag commands exactly as hotfix/SKILL.md writes them (steps 2 and 4).
// TestHotfixSkill_AutoRoute pins that the skill carries each one verbatim;
// TestHotfixTagCommands runs them against a scratch repo (SC-7).
const (
	cmdCur      = `CUR=$(git -C <repo> tag --merged "$REF" --list 'v[0-9]*' --sort=-v:refname | head -1)`
	cmdLatest   = `LATEST=$(git -C <repo> tag --list 'v[0-9]*' --sort=-v:refname | head -1)`
	cmdPrev     = `PREV=$(git -C <repo> tag --merged "$REF" --list 'v[0-9]*' --sort=-v:refname | sed -n 2p)`
	cmdIntro    = `C=$(git -C <repo> log -S'<faulty line>' --format=%H --reverse "$REF" -- <file> | head -1)`
	cmdAncestor = `git -C <repo> merge-base --is-ancestor "$C" "$PREV"`
)

// FR-2, FR-3.1, FR-4.1-4.3, FR-5.1, FR-7.2, SC-2, SC-5.
func TestHotfixSkill_AutoRoute(t *testing.T) {
	s := syncedAsset(t, "skills/hotfix/SKILL.md")
	if strings.Contains(s, "disable-model-invocation") || strings.Contains(s, "User-invoked only") || strings.Contains(s, "never run it automatically") {
		t.Error("hotfix must be model-invocable (FR-2)")
	}
	assertContainsAll(t, "hotfix/SKILL.md", s, []string{
		cmdCur, cmdLatest, cmdPrev, cmdIntro, cmdAncestor,
		`git diff --name-only "$REF"...HEAD`, "> 3",
		"git -C <repo> fetch origin --tags", "git -C <repo> tag --contains", "main → staging",
		"handed off from `/fix`",
		"Decide the response",
		"cannot pin the faulty line",
	})
	if strings.Contains(s, "create the PR against the resolved") || strings.Contains(s, "creatordate") {
		t.Error("stale step 10 / creatordate sort left in hotfix")
	}
	w := syncedAsset(t, "skills/hotfix/references/why-these-rules.md")
	assertContainsAll(t, "hotfix why-these-rules", w, []string{"rename", "-v:refname", "Clean checks", "How much diagnosis"})
}

// FR-1, FR-8, SC-4: /fix hands a production bug to /hotfix after Step 2.
func TestFixSkill_RoutesProdToHotfix(t *testing.T) {
	s := syncedAsset(t, "skills/fix/SKILL.md")
	assertContainsAll(t, "fix/SKILL.md", s, []string{
		"## Step 2b", "Skill(znf:hotfix)", "zenify hotfix baseref", "git show", "do not re-diagnose",
		"references/prd-routing.md",
	})
	if len(s) > 8192 || strings.Count(s, "\n") > 200 {
		t.Fatalf("fix over cap: %d bytes", len(s))
	}
	r := syncedAsset(t, "skills/fix/references/prd-routing.md")
	assertContainsAll(t, "prd-routing.md", r, []string{
		"cannot be pinned", "Skill(znf:hotfix)", "not on the hotfix base", "prd does not run this code yet",
		"track id:", "staging", "standalone", "path is missing",
	})
}

// SC-7: the skill's own tag commands pick fix-forward vs ask correctly.
func TestHotfixTagCommands(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}

	type result struct {
		cur, latest, prev, c string
		ancestorOK           bool
	}
	// decide mirrors the skill's rule: fix forward only when every value is
	// present, CUR == LATEST, and the faulty commit is an ancestor of PREV.
	decide := func(r result) string {
		if r.cur == "" || r.cur != r.latest || r.prev == "" || r.c == "" || !r.ancestorOK {
			return "ask"
		}
		return "fix forward"
	}

	cases := []struct {
		name         string
		build        func(g func(...string))
		want         result
		wantDecision string
	}{
		{
			name: "old-bug",
			build: func(g func(...string)) {
				commit(g, "bug.txt", "BUGLINE\n", "bug")
				commit(g, "a.txt", "1\n", "a")
				g("tag", "v260101.0")
				commit(g, "b.txt", "2\n", "b")
				g("tag", "v260201.0")
				commit(g, "c.txt", "3\n", "c")
				g("tag", "v260301.0")
			},
			want:         result{cur: "v260301.0", latest: "v260301.0", prev: "v260201.0", ancestorOK: true},
			wantDecision: "fix forward",
		},
		{
			name: "new-bug",
			build: func(g func(...string)) {
				commit(g, "a.txt", "1\n", "a")
				g("tag", "v260101.0")
				commit(g, "b.txt", "2\n", "b")
				g("tag", "v260201.0")
				commit(g, "bug.txt", "BUGLINE\n", "bug")
				g("tag", "v260301.0")
			},
			want:         result{cur: "v260301.0", latest: "v260301.0", prev: "v260201.0", ancestorOK: false},
			wantDecision: "ask",
		},
		{
			name: "no-tags",
			build: func(g func(...string)) {
				commit(g, "bug.txt", "BUGLINE\n", "bug")
				commit(g, "a.txt", "1\n", "a")
			},
			want:         result{},
			wantDecision: "ask",
		},
		{
			name: "latest-not-merged",
			build: func(g func(...string)) {
				commit(g, "bug.txt", "BUGLINE\n", "bug")
				g("tag", "v260101.0")
				commit(g, "b.txt", "2\n", "b")
				g("tag", "v260201.0")
				commit(g, "c.txt", "3\n", "c")
				g("tag", "v260301.0")
				g("checkout", "-q", "-b", "side")
				commit(g, "d.txt", "4\n", "d")
				g("tag", "v260401.0")
				g("checkout", "-q", "main")
			},
			want:         result{cur: "v260301.0", latest: "v260401.0", prev: "v260201.0", ancestorOK: true},
			wantDecision: "ask",
		},
		{
			name: "one-tag",
			build: func(g func(...string)) {
				commit(g, "bug.txt", "BUGLINE\n", "bug")
				g("tag", "v260101.0")
			},
			want:         result{cur: "v260101.0", latest: "v260101.0"},
			wantDecision: "ask",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			// The skill runs from the workspace root, which is not a git repo:
			// run the commands from a non-git sibling and reach the repo via -C.
			cwd := t.TempDir()
			sub := strings.NewReplacer("<repo>", dir, "<faulty line>", "BUGLINE", "<file>", "bug.txt").Replace
			script := "REF=main\n" + sub(cmdCur) + "\n" + sub(cmdLatest) + "\n" + sub(cmdPrev) + "\n" + sub(cmdIntro) + "\n" +
				`printf 'CUR=%s\nLATEST=%s\nPREV=%s\nC=%s\n' "$CUR" "$LATEST" "$PREV" "$C"` + "\n" +
				sub(cmdAncestor) + " 2>/dev/null && echo ANCESTOR=ok || echo ANCESTOR=fail\n"
			g := func(args ...string) {
				t.Helper()
				cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
				cmd.Dir = dir
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, out)
				}
			}
			g("init", "-q", "-b", "main")
			tc.build(func(a ...string) {
				if a[0] == "__write" {
					if err := os.WriteFile(filepath.Join(dir, a[1]), []byte(a[2]), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				g(a...)
			})

			cmd := exec.Command("bash", "-c", script)
			cmd.Dir = cwd
			cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CEILING_DIRECTORIES="+filepath.Dir(cwd))
			if out, err := exec.Command("git", "-C", cwd, "rev-parse", "--git-dir").CombinedOutput(); err == nil {
				t.Fatalf("cwd %s must not be a git repo: %s", cwd, out)
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("script: %v\n%s", err, out)
			}
			got := result{}
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				k, v, _ := strings.Cut(line, "=")
				switch k {
				case "CUR":
					got.cur = v
				case "LATEST":
					got.latest = v
				case "PREV":
					got.prev = v
				case "C":
					got.c = v
				case "ANCESTOR":
					got.ancestorOK = v == "ok"
				}
			}
			if got.cur != tc.want.cur || got.latest != tc.want.latest || got.prev != tc.want.prev {
				t.Errorf("tags = CUR %q LATEST %q PREV %q, want %q %q %q\n%s", got.cur, got.latest, got.prev, tc.want.cur, tc.want.latest, tc.want.prev, out)
			}
			if got.c == "" {
				t.Errorf("C empty, want the BUGLINE commit\n%s", out)
			}
			if tc.want.prev != "" && got.ancestorOK != tc.want.ancestorOK {
				t.Errorf("ancestor = %v, want %v\n%s", got.ancestorOK, tc.want.ancestorOK, out)
			}
			if d := decide(got); d != tc.wantDecision {
				t.Errorf("decision = %q, want %q\n%s", d, tc.wantDecision, out)
			}
		})
	}
}

// commit writes one file and commits it, through the case's git helper.
func commit(g func(...string), file, body, msg string) {
	g("__write", file, body)
	g("add", file)
	g("commit", "-q", "-m", msg)
}
