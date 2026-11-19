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
	const in = `[{"text":"Khách đã huỷ đơn có được đặt lại không?","situation":"Luật hiện tại chưa nói gì về đơn đã huỷ.","options":[{"label":"Cho đặt lại","detail":"Khách bấm đặt lại ngay trên đơn cũ.","consequence":"Đơn cũ mở lại, báo cáo đếm hai lần."},{"label":"Giữ nguyên giới hạn ngày","detail":"Chỉ đặt lại trong 7 ngày.","consequence":"Sau 7 ngày khách phải tạo đơn mới."}],"recommended":1,"default_if_silent":0,"audience":"business"}]` //znf:allow-lang
	if err := os.WriteFile(qf, []byte(in), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runAwait(t, "await", "question", "--summary", "s", "--questions", qf)
	if exitcode.Code(err) != exitcode.Awaiting {
		t.Fatalf("code = %d (%v), want 8", exitcode.Code(err), err)
	}
	r, _ := readRecord(t, home, strings.TrimSpace(out))
	if r.Kind != "question" || len(r.Questions) != 1 {
		t.Fatalf("record = %+v", r)
	}
	q := r.Questions[0]
	if q.Text != "Khách đã huỷ đơn có được đặt lại không?" || q.Situation != "Luật hiện tại chưa nói gì về đơn đã huỷ." || q.Audience != "business" || q.Recommended != 1 { //znf:allow-lang
		t.Errorf("q = %+v", q)
	}
	if q.DefaultIfSilent == nil || *q.DefaultIfSilent != 0 {
		t.Errorf("default_if_silent = %v, want pointer to 0", q.DefaultIfSilent)
	}
	if len(q.Options) != 2 || q.Options[1] != (awaitOption{Label: "Giữ nguyên giới hạn ngày", Detail: "Chỉ đặt lại trong 7 ngày.", Consequence: "Sau 7 ngày khách phải tạo đơn mới."}) { //znf:allow-lang
		t.Errorf("options = %+v", q.Options)
	}
	// Wire shape zenie reads, field names frozen.
	raw, _ := os.ReadFile(filepath.Join(home, "await", strings.TrimSpace(out)+".json"))
	var m struct {
		Questions []map[string]any `json:"questions"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"text", "situation", "options", "recommended", "default_if_silent", "audience"} {
		if _, ok := m.Questions[0][k]; !ok {
			t.Errorf("wire question lacks %q: %v", k, m.Questions[0])
		}
	}
	if o := m.Questions[0]["options"].([]any)[0].(map[string]any); o["label"] != "Cho đặt lại" || o["detail"] == nil || o["consequence"] == nil { //znf:allow-lang
		t.Errorf("wire option = %v", o)
	}
}

func TestAwaitQuestionDefaultIfSilentNull(t *testing.T) {
	home := awaitEnv(t)
	chdir(t, t.TempDir())
	qf := filepath.Join(t.TempDir(), "q.json")
	if err := os.WriteFile(qf, []byte(`[`+awaitQ(`"default_if_silent":null`)+`]`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runAwait(t, "await", "question", "--summary", "s", "--questions", qf)
	if exitcode.Code(err) != exitcode.Awaiting {
		t.Fatalf("code = %d (%v), want 8", exitcode.Code(err), err)
	}
	r, _ := readRecord(t, home, strings.TrimSpace(out))
	if r.Questions[0].DefaultIfSilent != nil {
		t.Errorf("default_if_silent = %v, want nil", *r.Questions[0].DefaultIfSilent)
	}
	raw, _ := os.ReadFile(filepath.Join(home, "await", strings.TrimSpace(out)+".json"))
	var m struct {
		Questions []map[string]any `json:"questions"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if v, ok := m.Questions[0]["default_if_silent"]; !ok || v != nil {
		t.Errorf("wire default_if_silent = %#v (present=%v), want null", v, ok)
	}
}

// awaitOpt builds one option JSON object.
func awaitOpt(label, detail, consequence string) string {
	return `{"label":"` + label + `","detail":"` + detail + `","consequence":"` + consequence + `"}`
}

// awaitQ builds a valid question with two options; each override is a
// `"key":value` fragment that replaces the same key in the base.
func awaitQ(overrides ...string) string {
	fields := map[string]string{
		"text":              `"q"`,
		"situation":         `"s"`,
		"options":           `[` + awaitOpt("a", "d", "c") + `,` + awaitOpt("b", "d", "c") + `]`,
		"recommended":       `0`,
		"default_if_silent": `null`,
		"audience":          `"business"`,
	}
	extra := ""
	for _, o := range overrides {
		k, v, _ := strings.Cut(o, ":")
		k = strings.Trim(k, `"`)
		if _, ok := fields[k]; ok {
			fields[k] = v
		} else {
			extra = "," + o
		}
	}
	return `{"text":` + fields["text"] + `,"situation":` + fields["situation"] + `,"options":` + fields["options"] +
		`,"recommended":` + fields["recommended"] + `,"default_if_silent":` + fields["default_if_silent"] +
		`,"audience":` + fields["audience"] + extra + `}`
}

func TestAwaitQuestionAcceptBoundary(t *testing.T) {
	home := awaitEnv(t)
	chdir(t, t.TempDir())
	four := `[` + awaitOpt("a", "d", "c") + `,` + awaitOpt("b", "d", "c") + `,` + awaitOpt("c", "d", "c") + `,` + awaitOpt("d", "d", "c") + `]`
	label24 := strings.Repeat("ữ", 24)                                                                                               // 24 runes, 72 bytes //znf:allow-lang
	in := `[` + awaitQ(`"options":`+four, `"recommended":3`, `"default_if_silent":3`, `"text":"`+strings.Repeat("ế", 300)+`"`) + `]` //znf:allow-lang
	in = strings.Replace(in, `"label":"a"`, `"label":"`+label24+`"`, 1)
	qf := filepath.Join(t.TempDir(), "q.json")
	if err := os.WriteFile(qf, []byte(in), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runAwait(t, "await", "question", "--summary", "s", "--questions", qf)
	if exitcode.Code(err) != exitcode.Awaiting {
		t.Fatalf("code = %d (%v), want 8", exitcode.Code(err), err)
	}
	r, _ := readRecord(t, home, strings.TrimSpace(out))
	q := r.Questions[0]
	if len(q.Options) != 4 || q.Recommended != 3 || q.DefaultIfSilent == nil || *q.DefaultIfSilent != 3 || q.Options[0].Label != label24 {
		t.Fatalf("record = %+v", r)
	}
}

func TestAwaitRefusals(t *testing.T) {
	q := []string{"await", "question", "--summary", "s"}
	cases := []struct {
		name string
		env  string
		args []string
		qs   string // when set, written to a file and passed as --questions
		want string // when set, the error must contain it
	}{
		{"not autonomous", "", []string{"await", "spec", "--summary", "s"}, "", ""},
		{"bad kind", "1", []string{"await", "deploy", "--summary", "s"}, "", ""},
		{"missing file", "1", []string{"await", "spec", "--summary", "s", "--file", "nope.md"}, "", ""},
		{"no summary", "1", []string{"await", "spec"}, "", ""},
		{"blank summary", "1", []string{"await", "plan", "--summary", " \t"}, "", ""},
		{"data-write without command", "1", []string{"await", "data-write", "--summary", "s"}, "", ""},
		{"data-write blank command", "1", []string{"await", "data-write", "--summary", "s", "--command", "  "}, "", ""},
		{"data-write other command", "1", []string{"await", "data-write", "--summary", "s", "--command", "zenify data run --stg --write -- node x.js"}, "", ""},
		{"question without --questions", "1", []string{"await", "question", "--summary", "s"}, "", ""},
		{"two questions", "1", q, `[` + awaitQ() + `,` + awaitQ() + `]`, "want exactly 1"},
		{"zero questions", "1", q, `[]`, "want exactly 1"},
		{"missing situation", "1", q, `[` + awaitQ(`"situation":""`) + `]`, "situation"},
		{"one option", "1", q, `[` + awaitQ(`"options":[`+awaitOpt("a", "d", "c")+`]`, `"recommended":0`) + `]`, "options"},
		{"five options", "1", q, `[` + awaitQ(`"options":[`+strings.Repeat(awaitOpt("a", "d", "c")+",", 4)+awaitOpt("a", "d", "c")+`]`) + `]`, "options"},
		{"label 25 runes", "1", q, `[` + awaitQ(`"options":[`+awaitOpt("Giữ nguyên giới hạn ngày!", "d", "c")+`,`+awaitOpt("b", "d", "c")+`]`) + `]`, "label"}, //znf:allow-lang
		{"empty label", "1", q, `[` + awaitQ(`"options":[`+awaitOpt("", "d", "c")+`,`+awaitOpt("b", "d", "c")+`]`) + `]`, "label"},
		{"empty detail", "1", q, `[` + awaitQ(`"options":[`+awaitOpt("a", "", "c")+`,`+awaitOpt("b", "d", "c")+`]`) + `]`, "detail"},
		{"empty consequence", "1", q, `[` + awaitQ(`"options":[`+awaitOpt("a", "d", "")+`,`+awaitOpt("b", "d", "c")+`]`) + `]`, "consequence"},
		{"recommended too high", "1", q, `[` + awaitQ(`"recommended":5`) + `]`, "recommended"},
		{"recommended negative", "1", q, `[` + awaitQ(`"recommended":-1`) + `]`, "recommended"},
		{"default_if_silent out of range", "1", q, `[` + awaitQ(`"options":[`+awaitOpt("a", "d", "c")+`,`+awaitOpt("b", "d", "c")+`,`+awaitOpt("c", "d", "c")+`]`, `"default_if_silent":4`) + `]`, "default_if_silent"},
		{"text 301 runes", "1", q, `[` + awaitQ(`"text":"`+strings.Repeat("ế", 301)+`"`) + `]`, "text"}, //znf:allow-lang
		{"blank question text", "1", q, `[` + awaitQ(`"text":"  "`) + `]`, "text"},
		{"bad audience", "1", q, `[` + awaitQ(`"audience":"manager"`) + `]`, "audience"},
		{"unknown field", "1", q, `[` + awaitQ(`"foo":1`) + `]`, "foo"},
		{"old shape: options are strings", "1", q, `[{"text":"q","options":["a","b"],"audience":"business"}]`, "JSON array of one"},
		{"not json", "1", q, `{`, "JSON array of one"},
		{"trailing data", "1", q, `[` + awaitQ() + `] {}`, "unexpected data"},
		{"--questions on spec", "1", []string{"await", "spec", "--summary", "s"}, `[` + awaitQ() + `]`, ""},
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
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q lacks %q", err, tc.want)
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

func TestAwaitEndTurn(t *testing.T) {
	const wantLine = "await recorded: end your turn now and do nothing else"
	cases := []struct {
		name       string
		autonomous string
		await      string
		wantErr    string // substring of the error; "" with wantCode 0 = no error
		wantCode   int
		wantRecs   int
	}{
		{"not autonomous", "", "end-turn", "only works in autonomous mode", exitcode.BadArgs, 0},
		{"autonomous without ZNF_AWAIT", "1", "", "", exitcode.Awaiting, 1},
		{"autonomous end-turn", "1", "end-turn", "", 0, 1},
		{"autonomous other value", "1", "other", "", exitcode.Awaiting, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := awaitEnv(t)
			t.Setenv("ZNF_AUTONOMOUS", tc.autonomous)
			t.Setenv("ZNF_AWAIT", tc.await)
			chdir(t, t.TempDir())
			out, err := runAwait(t, "await", "needs-human", "--summary", "s")
			if got := exitcode.Code(err); got != tc.wantCode {
				t.Fatalf("code = %d (%v), want %d", got, err, tc.wantCode)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
			ents, _ := os.ReadDir(filepath.Join(home, "await"))
			if len(ents) != tc.wantRecs {
				t.Fatalf("records = %d, want %d", len(ents), tc.wantRecs)
			}
			if tc.name == "autonomous end-turn" {
				lines := strings.Split(strings.TrimSpace(out), "\n")
				if len(lines) != 2 || lines[1] != wantLine {
					t.Fatalf("stdout lines = %q, want id then %q", lines, wantLine)
				}
			}
		})
	}
}

func TestBuildAwaitRecordHandoffTo(t *testing.T) {
	awaitEnv(t)
	chdir(t, t.TempDir())
	rec, err := buildAwaitRecord("handoff", "s", "", "", " Tân ", nil) //znf:allow-lang
	if err != nil {
		t.Fatal(err)
	}
	if rec.Kind != "handoff" || rec.To != "Tân" { //znf:allow-lang
		t.Fatalf("rec = %+v", rec)
	}
	b, _ := json.Marshal(rec)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["to"] != "Tân" { //znf:allow-lang
		t.Errorf(`json "to" = %#v`, m["to"])
	}
	other, err := buildAwaitRecord("needs-human", "s", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	ob, _ := json.Marshal(other)
	var om map[string]any
	_ = json.Unmarshal(ob, &om)
	if _, has := om["to"]; has {
		t.Errorf("non-handoff record has to: %s", ob)
	}
}

func TestBuildAwaitRecordToRefusals(t *testing.T) {
	awaitEnv(t)
	chdir(t, t.TempDir())
	cases := []struct{ kind, to, want string }{
		{"handoff", "", "--to"},
		{"handoff", "  ", "--to"},
		{"needs-human", "X", "--to is only for kind handoff"},
		{"handoff", "Tan\nrm -rf", "printable characters only"},
		{"handoff", "Tan\u2028x", "printable characters only"},
		{"handoff", "Tan\u202ex", "printable characters only"},
		{"handoff", strings.Repeat("a", 101), "at most 100"},
	}
	for _, tc := range cases {
		_, err := buildAwaitRecord(tc.kind, "s", "", "", tc.to, nil)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s/%q: err = %v, want contains %q", tc.kind, tc.to, err, tc.want)
		}
	}
}

func TestAwaitHandoffCLI(t *testing.T) {
	home := awaitEnv(t)
	chdir(t, t.TempDir())
	out, err := runAwait(t, "await", "handoff", "--summary", "s", "--to", "@tan")
	if exitcode.Code(err) != exitcode.Awaiting {
		t.Fatalf("code = %d (%v), want 8", exitcode.Code(err), err)
	}
	r, _ := readRecord(t, home, strings.TrimSpace(out))
	if r.To != "@tan" {
		t.Errorf("to = %q", r.To)
	}
}
