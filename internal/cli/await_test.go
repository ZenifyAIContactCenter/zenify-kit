package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// runAwait runs the root command with args and returns stdout and the error.
func runAwait(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func awaitEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ZENIFY_HOME", home)
	t.Setenv("ZNF_AUTONOMOUS", "1")
	t.Setenv("ZNF_TASK_ID", "task-42")
	return home
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range [][]string{{"init", "-q", "-b", "feat-x"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "i"}} {
		c := exec.Command("git", a...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", a, err, out)
		}
	}
	return dir
}

func readRecord(t *testing.T, home, id string) (awaitRecord, os.FileInfo) {
	t.Helper()
	p := filepath.Join(home, "await", id+".json")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	var r awaitRecord
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r, st
}

func TestAwaitSpecInGitRepo(t *testing.T) {
	home := awaitEnv(t)
	repo := gitRepo(t)
	chdir(t, repo)
	spec := filepath.Join(repo, "spec.md")
	_ = os.WriteFile(spec, []byte("x"), 0o644)

	out, err := runAwait(t, "await", "spec", "--summary", "s", "--file", "spec.md")
	if exitcode.Code(err) != exitcode.Awaiting {
		t.Fatalf("code = %d (%v), want 8", exitcode.Code(err), err)
	}
	id := strings.TrimSuffix(out, "\n")
	if !regexp.MustCompile(`^spec-\d{8}T\d{6}Z-[0-9a-f]{8}$`).MatchString(id) {
		t.Fatalf("stdout %q is not <id>\\n", out)
	}
	r, st := readRecord(t, home, id)
	if r.Kind != "spec" || r.ID != id || r.Summary != "s" || r.Task != "task-42" {
		t.Errorf("record = %+v", r)
	}
	if len(r.Files) != 1 || r.Files[0] != spec {
		t.Errorf("files = %v, want [%s]", r.Files, spec)
	}
	if r.Repo != repo || r.Branch != "feat-x" || r.Cwd != repo || r.CreatedAt.IsZero() {
		t.Errorf("repo/branch/cwd/created_at = %q %q %q %v", r.Repo, r.Branch, r.Cwd, r.CreatedAt)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", st.Mode().Perm())
	}
	dst, _ := os.Stat(filepath.Join(home, "await"))
	if dst.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v, want 0700", dst.Mode().Perm())
	}
}

func TestAwaitOutsideGit(t *testing.T) {
	home := awaitEnv(t)
	chdir(t, t.TempDir())
	out, err := runAwait(t, "await", "needs-human", "--summary", "s")
	if exitcode.Code(err) != exitcode.Awaiting {
		t.Fatalf("code = %d (%v)", exitcode.Code(err), err)
	}
	r, _ := readRecord(t, home, strings.TrimSpace(out))
	if r.Repo != "" || r.Branch != "" {
		t.Errorf("repo/branch = %q/%q, want empty", r.Repo, r.Branch)
	}
}

func TestAwaitDataWriteCommand(t *testing.T) {
	home := awaitEnv(t)
	chdir(t, t.TempDir())
	const c = "zenify data run --prd --write -- node x.js"
	out, err := runAwait(t, "await", "data-write", "--summary", "s", "--command", c)
	if exitcode.Code(err) != exitcode.Awaiting {
		t.Fatalf("code = %d (%v)", exitcode.Code(err), err)
	}
	r, _ := readRecord(t, home, strings.TrimSpace(out))
	if r.Command != c {
		t.Errorf("command = %q", r.Command)
	}
}

// The record is renamed into place from .<id>.json.tmp: nothing else is left
// in the await dir for the host to pick up.
func TestAwaitWritesOnlyTheRecord(t *testing.T) {
	home := awaitEnv(t)
	chdir(t, t.TempDir())
	out, err := runAwait(t, "await", "needs-human", "--summary", "s")
	if exitcode.Code(err) != exitcode.Awaiting {
		t.Fatalf("code = %d (%v)", exitcode.Code(err), err)
	}
	ents, _ := os.ReadDir(filepath.Join(home, "await"))
	if len(ents) != 1 || ents[0].Name() != strings.TrimSpace(out)+".json" {
		t.Fatalf("await dir = %v, want only %s.json", ents, strings.TrimSpace(out))
	}
}

// The record goes through an exclusive .<id>.json.tmp: a stale temp file of
// the same id is an error, and no half-written record appears.
func TestWriteAwaitRecordViaExclusiveTemp(t *testing.T) {
	home := awaitEnv(t)
	dir := filepath.Join(home, "await")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, ".plan-x.json.tmp")
	if err := os.WriteFile(tmp, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeAwaitRecord(awaitRecord{ID: "plan-x", Kind: "plan", Summary: "s"}); err == nil {
		t.Fatal("want an error while .plan-x.json.tmp exists")
	}
	if _, err := os.Stat(filepath.Join(dir, "plan-x.json")); err == nil {
		t.Fatal("record written despite the temp-file failure")
	}
	_ = os.Remove(tmp)
	if err := writeAwaitRecord(awaitRecord{ID: "plan-x", Kind: "plan", Summary: "s"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tmp); err == nil {
		t.Fatal("temp file left behind")
	}
}

// task is recorded only when ZNF_TASK_ID is a plain id, as the broker header.
func TestAwaitTaskOnlyWhenValid(t *testing.T) {
	home := awaitEnv(t)
	chdir(t, t.TempDir())
	t.Setenv("ZNF_TASK_ID", "bad value!")
	out, _ := runAwait(t, "await", "plan", "--summary", "s")
	if r, _ := readRecord(t, home, strings.TrimSpace(out)); r.Task != "" {
		t.Errorf("task = %q, want empty for an invalid id", r.Task)
	}
}

// The command `data run` prints when it stops an agent's prd write passes the
// data-write check as is.
func TestAwaitAcceptsLeadWriteCmd(t *testing.T) {
	home := awaitEnv(t)
	chdir(t, t.TempDir())
	c := leadWriteCmd("/abs/repo dir", "prd", true, []string{"node", "x.js", "--tenant", "a b"})
	out, err := runAwait(t, "await", "data-write", "--summary", "s", "--command", c)
	if exitcode.Code(err) != exitcode.Awaiting {
		t.Fatalf("code = %d (%v)", exitcode.Code(err), err)
	}
	if r, _ := readRecord(t, home, strings.TrimSpace(out)); r.Command != c {
		t.Errorf("command = %q, want %q", r.Command, c)
	}
}

func TestAwaitQuestionRecord(t *testing.T) {
	home := awaitEnv(t)
	chdir(t, t.TempDir())
	qf := filepath.Join(t.TempDir(), "q.json")
	const in = `[{"text":"Who sees this?","options":["agents","admins","everyone"],"audience":"business"},{"text":"Which index?","options":[],"audience":"technical"}]`
	if err := os.WriteFile(qf, []byte(in), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runAwait(t, "await", "question", "--summary", "s", "--questions", qf)
	if exitcode.Code(err) != exitcode.Awaiting {
		t.Fatalf("code = %d (%v), want 8", exitcode.Code(err), err)
	}
	r, _ := readRecord(t, home, strings.TrimSpace(out))
	if r.Kind != "question" || len(r.Questions) != 2 {
		t.Fatalf("record = %+v", r)
	}
	if q := r.Questions[0]; q.Text != "Who sees this?" || q.Audience != "business" || strings.Join(q.Options, ",") != "agents,admins,everyone" {
		t.Errorf("q0 = %+v", q)
	}
	if q := r.Questions[1]; q.Text != "Which index?" || q.Audience != "technical" || len(q.Options) != 0 {
		t.Errorf("q1 = %+v", q)
	}
	// Wire shape zenie reads: top-level "questions", each with text/options/audience;
	// options is [] (not null) for a free-text question.
	raw, _ := os.ReadFile(filepath.Join(home, "await", strings.TrimSpace(out)+".json"))
	var m struct {
		Questions []map[string]any `json:"questions"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if o, ok := m.Questions[1]["options"].([]any); !ok || len(o) != 0 {
		t.Errorf("options = %#v, want []", m.Questions[1]["options"])
	}
}

func TestAwaitRefusals(t *testing.T) {
	cases := []struct {
		name string
		env  string
		args []string
		qs   string // when set, written to a file and passed as --questions
	}{
		{"not autonomous", "", []string{"await", "spec", "--summary", "s"}, ""},
		{"bad kind", "1", []string{"await", "deploy", "--summary", "s"}, ""},
		{"missing file", "1", []string{"await", "spec", "--summary", "s", "--file", "nope.md"}, ""},
		{"no summary", "1", []string{"await", "spec"}, ""},
		{"blank summary", "1", []string{"await", "plan", "--summary", " \t"}, ""},
		{"data-write without command", "1", []string{"await", "data-write", "--summary", "s"}, ""},
		{"data-write blank command", "1", []string{"await", "data-write", "--summary", "s", "--command", "  "}, ""},
		{"data-write other command", "1", []string{"await", "data-write", "--summary", "s", "--command", "zenify data run --stg --write -- node x.js"}, ""},
		{"question without --questions", "1", []string{"await", "question", "--summary", "s"}, ""},
		{"six questions", "1", []string{"await", "question", "--summary", "s"}, `[` + strings.Repeat(`{"text":"q","options":[],"audience":"business"},`, 5) + `{"text":"q","options":[],"audience":"business"}]`},
		{"zero questions", "1", []string{"await", "question", "--summary", "s"}, `[]`},
		{"five options", "1", []string{"await", "question", "--summary", "s"}, `[{"text":"q","options":["a","b","c","d","e"],"audience":"business"}]`},
		{"blank question text", "1", []string{"await", "question", "--summary", "s"}, `[{"text":"  ","options":[],"audience":"business"}]`},
		{"blank option", "1", []string{"await", "question", "--summary", "s"}, `[{"text":"q","options":["a",""],"audience":"business"}]`},
		{"bad audience", "1", []string{"await", "question", "--summary", "s"}, `[{"text":"q","options":[],"audience":"manager"}]`},
		{"not json", "1", []string{"await", "question", "--summary", "s"}, `{`},
		{"--questions on spec", "1", []string{"await", "spec", "--summary", "s"}, `[{"text":"q","options":[],"audience":"business"}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := awaitEnv(t)
			t.Setenv("ZNF_AUTONOMOUS", tc.env)
			chdir(t, t.TempDir())
			args := tc.args
			if tc.qs != "" {
				qf := filepath.Join(t.TempDir(), "q.json")
				if err := os.WriteFile(qf, []byte(tc.qs), 0o600); err != nil {
					t.Fatal(err)
				}
				args = append(append([]string{}, args...), "--questions", qf)
			}
			_, err := runAwait(t, args...)
			if exitcode.Code(err) != exitcode.BadArgs {
				t.Fatalf("code = %d (%v), want BadArgs", exitcode.Code(err), err)
			}
			if _, e := os.Stat(filepath.Join(home, "await")); e == nil {
				ents, _ := os.ReadDir(filepath.Join(home, "await"))
				if len(ents) != 0 {
					t.Errorf("record written: %v", ents)
				}
			}
		})
	}
}

func TestAwaitShow(t *testing.T) {
	home := awaitEnv(t)
	chdir(t, t.TempDir())
	out, _ := runAwait(t, "await", "plan", "--summary", "s")
	id := strings.TrimSpace(out)
	want, err := os.ReadFile(filepath.Join(home, "await", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := runAwait(t, "await", "show", id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != strings.TrimSpace(string(want)) {
		t.Errorf("show = %q, want %q", got, want)
	}
	_, err = runAwait(t, "await", "show", "plan-nope")
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("unknown id code = %d, want BadArgs", exitcode.Code(err))
	}
	_, err = runAwait(t, "await", "show", "../x")
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("path-ish id code = %d, want BadArgs", exitcode.Code(err))
	}
}
