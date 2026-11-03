package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The shell lines exactly as the skills write them: hotfix steps 2, 4 and 9,
// and fix/references/prd-routing.md. The Auto/Routes tests pin that each skill
// carries them verbatim; TestHotfixTagCommands, TestHotfixSizeCount and
// TestPrdRoutingCommands run them against scratch repos (SC-7).
const (
	cmdCur      = `CUR=$(git -C <repo> tag --merged "$REF" --list 'v[0-9]*' --sort=-v:refname | head -1)`
	cmdLatest   = `LATEST=$(git -C <repo> tag --list 'v[0-9]*' --sort=-v:refname | head -1)`
	cmdPrev     = `PREV=$(git -C <repo> tag --merged "$REF" --list 'v[0-9]*' --sort=-v:refname | sed -n 2p)`
	cmdLineNo   = `N=$(git -C <repo> show "$REF:<file>" | grep -nF -f <needle> | cut -d: -f1)`
	cmdBlame    = `C=$(case "$N" in (''|*[!0-9]*) ;; (*) git -C <repo> blame -L"$N,$N" --porcelain "$REF" -- <file> | head -1 | cut -d' ' -f1 ;; esac)`
	cmdPrint4   = `printf 'PREV=%s N=%s C=%s\n' "$PREV" "$N" "$C"`
	cmdAncestor = `git -C <repo> merge-base --is-ancestor "$C" "$PREV"`
	cmdSize     = `B=$(git -C <worktree> merge-base <ref> HEAD) && { git -C <worktree> diff --name-only "$B"; git -C <worktree> ls-files --others --exclude-standard; } | sort -u | wc -l`
	cmdOnBase   = `git -C <repo> cat-file -e "$REF:<file>" || echo PATH_MISSING`
	cmdGrepBase = `git -C <repo> show "$REF:<file>" | grep -nF -f <needle>`
)

// FR-2, FR-3.1, FR-4.1-4.3, FR-5.1, FR-7.2, SC-2, SC-5.
func TestHotfixSkill_AutoRoute(t *testing.T) {
	s := syncedAsset(t, "skills/hotfix/SKILL.md")
	if strings.Contains(s, "disable-model-invocation") || strings.Contains(s, "User-invoked only") || strings.Contains(s, "never run it automatically") {
		t.Error("hotfix must be model-invocable (FR-2)")
	}
	assertContainsAll(t, "hotfix/SKILL.md", s, []string{
		cmdCur, cmdLatest, cmdPrev, cmdLineNo, cmdBlame, cmdPrint4, cmdAncestor, cmdSize, "No number printed, over 3",
		"git -C <repo> fetch origin --tags", "git -C <repo> tag --contains", "the repo's `baseRef`",
		"handed off from `/fix`", "first word after the skill name",
		"Decide the response",
		"cannot pin the faulty line", "never paste source", "reuse the printed values",
		"BASE=$(cd <repo> && zenify wt base)", "`REF` = `BASE`", "REF=<ref>   # step 2's printed ref",
		"--base <ref>", "re-run this count and `/gate`", "build check in `references/post-deploy-check.md`",
	})
	for _, stale := range []string{"create the PR against the resolved", "creatordate", "log -S'", "main → staging", `"$REF"...HEAD`, `--base "$REF"`} {
		if strings.Contains(s, stale) {
			t.Errorf("stale text %q left in hotfix", stale)
		}
	}
	w := syncedAsset(t, "skills/hotfix/references/why-these-rules.md")
	assertContainsAll(t, "hotfix why-these-rules", w, []string{"rename", "-v:refname", "Clean checks", "How much diagnosis", "git blame", "cannot see"})
}

// FR-1, FR-8, SC-4: /fix hands a production bug to /hotfix after Step 2.
func TestFixSkill_RoutesProdToHotfix(t *testing.T) {
	s := syncedAsset(t, "skills/fix/SKILL.md")
	assertContainsAll(t, "fix/SKILL.md", s, []string{
		"## Step 2b", "Skill(znf:hotfix)", "zenify hotfix baseref", "git show", "do not re-diagnose",
		"references/prd-routing.md", "`track id:`", "/fix ends", "no stop (`/hotfix` step 3)",
	})
	if len(s) > 8192 || strings.Count(s, "\n") > 200 {
		t.Fatalf("fix over cap: %d bytes", len(s))
	}
	r := syncedAsset(t, "skills/fix/references/prd-routing.md")
	assertContainsAll(t, "prd-routing.md", r, []string{
		"cannot be pinned", "Skill(znf:hotfix)", "No exact match on the hotfix base", "a failed match", "Tempo trace",
		"track id: <the id Step 0 opened>", "<slug>-hf", "/fix` ends after the call", "staging", "standalone", "path is missing",
		cmdOnBase, cmdGrepBase,
	})
}

// gitRepo returns a scratch repo and a git helper isolated from the
// developer's git config (hooks, templates, signing).
func gitRepo(t *testing.T) (string, func(...string)) {
	t.Helper()
	dir := t.TempDir()
	g := func(args ...string) {
		t.Helper()
		if args[0] == "__write" {
			p := filepath.Join(dir, args[1])
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(args[2]), 0o644); err != nil {
				t.Fatal(err)
			}
			return
		}
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
		cmd.Dir = dir
		cmd.Env = isolatedGitEnv(filepath.Dir(dir))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	g("init", "-q", "-b", "main")
	return dir, g
}

func isolatedGitEnv(ceiling string) []string {
	return append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CEILING_DIRECTORIES="+ceiling)
}

// runSkillScript runs script from a non-git directory, as the skill does from
// the workspace root, and returns its KEY=value lines.
func runSkillScript(t *testing.T, script string) (map[string]string, string) {
	t.Helper()
	cwd := t.TempDir()
	if out, err := exec.Command("git", "-C", cwd, "rev-parse", "--git-dir").CombinedOutput(); err == nil {
		t.Fatalf("cwd %s must not be a git repo: %s", cwd, out)
	}
	cmd := exec.Command("bash", "-c", script)
	cmd.Dir = cwd
	cmd.Env = isolatedGitEnv(filepath.Dir(cwd))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script: %v\n%s", err, out)
	}
	kv := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		for _, f := range strings.Fields(line) {
			if k, v, ok := strings.Cut(f, "="); ok {
				kv[k] = v
			}
		}
	}
	return kv, string(out)
}

func needAuto(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"git", "bash"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skip(bin + " not on PATH")
		}
	}
}

// SC-7: the skill's own tag and blame commands pick fix-forward vs ask correctly.
func TestHotfixTagCommands(t *testing.T) {
	needAuto(t)

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
	const bugline = "BUGLINE"

	cases := []struct {
		name         string
		file, needle string // defaults bug.txt, BUGLINE + "\n"
		build        func(g func(...string))
		want         result
		wantCEmpty   bool
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
		{
			// A second copy of an old line lands in the last deploy: log -S
			// would return the old commit; the needle matches twice → ask.
			name: "duplicate-added-after-prev",
			build: func(g func(...string)) {
				commit(g, "bug.txt", "x\nBUGLINE\n", "old copy")
				g("tag", "v260101.0")
				commit(g, "b.txt", "2\n", "b")
				g("tag", "v260201.0")
				commit(g, "bug.txt", "x\nBUGLINE\ny\nBUGLINE\n", "new copy")
				g("tag", "v260301.0")
			},
			want:         result{cur: "v260301.0", latest: "v260301.0", prev: "v260201.0"},
			wantCEmpty:   true,
			wantDecision: "ask",
		},
		{
			// Blame follows the rename back to the original commit.
			name: "old-bug-renamed-after-prev",
			build: func(g func(...string)) {
				commit(g, "old.txt", "a\nBUGLINE\n", "bug")
				g("tag", "v260101.0")
				commit(g, "b.txt", "2\n", "b")
				g("tag", "v260201.0")
				g("mv", "old.txt", "bug.txt")
				g("commit", "-q", "-m", "rename")
				g("tag", "v260301.0")
			},
			want:         result{cur: "v260301.0", latest: "v260301.0", prev: "v260201.0", ancestorOK: true},
			wantDecision: "fix forward",
		},
		{
			// A JS line with quotes and brackets: grep -F still finds it.
			name:   "quote-and-bracket-line",
			file:   "src/a.js",
			needle: "if (a['k'][0].*) return null;\n",
			build: func(g func(...string)) {
				commit(g, "src/a.js", "const a = {};\n  if (a['k'][0].*) return null;\n", "bug")
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
			// The file is on $REF but the needle matches nothing: N is empty and
			// must not reach blame -L"," (git reads that as the whole file).
			name:   "needle-no-match",
			needle: "NOT IN FILE\n",
			build: func(g func(...string)) {
				commit(g, "bug.txt", "BUGLINE\n", "bug")
				g("tag", "v260101.0")
				commit(g, "b.txt", "2\n", "b")
				g("tag", "v260201.0")
			},
			want:         result{cur: "v260201.0", latest: "v260201.0", prev: "v260101.0"},
			wantCEmpty:   true,
			wantDecision: "ask",
		},
		{
			name: "faulty-file-absent",
			file: "gone.txt",
			build: func(g func(...string)) {
				commit(g, "a.txt", "1\n", "a")
				g("tag", "v260101.0")
				commit(g, "b.txt", "2\n", "b")
				g("tag", "v260201.0")
			},
			want:         result{cur: "v260201.0", latest: "v260201.0", prev: "v260101.0"},
			wantCEmpty:   true,
			wantDecision: "ask",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file, needle := tc.file, tc.needle
			if file == "" {
				file = "bug.txt"
			}
			if needle == "" {
				needle = bugline + "\n"
			}
			dir, g := gitRepo(t)
			tc.build(g)
			needleFile := filepath.Join(t.TempDir(), "needle")
			if err := os.WriteFile(needleFile, []byte(needle), 0o644); err != nil {
				t.Fatal(err)
			}
			sub := strings.NewReplacer("<repo>", dir, "<needle>", needleFile, "<file>", file).Replace
			script := "REF=main\n" + sub(cmdCur) + "\n" + sub(cmdLatest) + "\n" + sub(cmdPrev) + "\n" +
				sub(cmdLineNo) + " 2>/dev/null\n" + sub(cmdBlame) + " 2>/dev/null\n" +
				`printf 'CUR=%s\nLATEST=%s\n' "$CUR" "$LATEST"` + "\n" + sub(cmdPrint4) + "\n" +
				sub(cmdAncestor) + " 2>/dev/null && echo ANCESTOR=ok || echo ANCESTOR=fail\n"
			kv, out := runSkillScript(t, script)
			got := result{cur: kv["CUR"], latest: kv["LATEST"], prev: kv["PREV"], c: kv["C"], ancestorOK: kv["ANCESTOR"] == "ok"}
			if got.cur != tc.want.cur || got.latest != tc.want.latest || got.prev != tc.want.prev {
				t.Errorf("tags = CUR %q LATEST %q PREV %q, want %q %q %q\n%s", got.cur, got.latest, got.prev, tc.want.cur, tc.want.latest, tc.want.prev, out)
			}
			if (got.c == "") != tc.wantCEmpty {
				t.Errorf("C = %q, want empty=%v\n%s", got.c, tc.wantCEmpty, out)
			}
			if tc.want.prev != "" && !tc.wantCEmpty && got.ancestorOK != tc.want.ancestorOK {
				t.Errorf("ancestor = %v, want %v\n%s", got.ancestorOK, tc.want.ancestorOK, out)
			}
			if d := decide(got); d != tc.wantDecision {
				t.Errorf("decision = %q, want %q\n%s", d, tc.wantDecision, out)
			}
		})
	}
}

// FR-5.1: step 9 counts committed, modified and untracked files before
// /ship commits, and ignores upstream commits fetched after branching.
func TestHotfixSizeCount(t *testing.T) {
	needAuto(t)
	dir, g := gitRepo(t)
	for _, f := range []string{"a.txt", "b.txt", "c.txt", "d.txt"} {
		commit(g, f, "0\n", f)
	}
	g("checkout", "-q", "-b", "hf")
	commit(g, "a.txt", "1\n", "committed fix")
	g("checkout", "-q", "main")
	commit(g, "d.txt", "upstream\n", "upstream after branching")
	g("checkout", "-q", "hf")
	g("__write", "b.txt", "1\n")
	g("__write", "c.txt", "1\n")
	g("__write", "new_test.txt", "1\n")
	for _, tc := range []struct{ ref, want string }{
		{"main", "4"}, // a committed, b+c modified, new_test untracked; d is upstream
		{"", ""},      // empty ref: merge-base fails, no number printed -> the skill asks
		{"nope", ""},
	} {
		cmd := strings.NewReplacer("<worktree>", dir, "<ref>", "'"+tc.ref+"'").Replace(cmdSize)
		_, out := runSkillScript(t, "{ "+cmd+"; } 2>/dev/null || true\n")
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("ref %q: count = %q, want %q", tc.ref, got, tc.want)
		}
	}
}

// FR-1.1: prd-routing's base lookup finds a quoted line and reports a missing path.
func TestPrdRoutingCommands(t *testing.T) {
	needAuto(t)
	dir, g := gitRepo(t)
	commit(g, "src/a.js", "x\n  if (a['k'][0]) return null;\n", "bug")
	needle := filepath.Join(t.TempDir(), "needle")
	if err := os.WriteFile(needle, []byte("if (a['k'][0]) return null;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ file, want string }{
		{"src/a.js", "2:"},
		{"src/moved.js", "PATH_MISSING"},
	} {
		sub := strings.NewReplacer("<repo>", dir, "<needle>", needle, "<file>", tc.file).Replace
		script := "REF=main\n" + sub(cmdOnBase) + "\n" + sub(cmdGrepBase) + " 2>/dev/null || true\n"
		_, out := runSkillScript(t, script)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: want %q in\n%s", tc.file, tc.want, out)
		}
	}
}

// commit writes one file and commits it, through the case's git helper.
func commit(g func(...string), file, body, msg string) {
	g("__write", file, body)
	g("add", file)
	g("commit", "-q", "-m", msg)
}

func TestNoManualHotfixRecipeOutsideHotfix(t *testing.T) {
	for _, rel := range []string{
		"skills/discipline/SKILL.md",
		"skills/discipline/references/worktree-rationale.md",
		"skills/discipline/references/rules-full-text.md",
	} {
		s := syncedAsset(t, rel)
		for _, bad := range []string{"--type hotfix --base", "BASE=$(zenify hotfix baseref", "overrides it to `zenify hotfix baseref`"} {
			if strings.Contains(s, bad) {
				t.Errorf("%s still carries the manual hotfix recipe %q", rel, bad)
			}
		}
		if rel != "skills/discipline/references/worktree-rationale.md" && !strings.Contains(s, "Step 2b") {
			t.Errorf("%s routing row must say /fix hands prd bugs over at Step 2b", rel)
		}
	}
	ti := syncedAsset(t, "skills/top-issues/SKILL.md")
	if strings.Contains(ti, "user-only") {
		t.Error("top-issues still calls /hotfix user-only")
	}
}
