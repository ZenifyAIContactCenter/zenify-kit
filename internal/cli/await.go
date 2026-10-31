package cli

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/brokerclient"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/spf13/cobra"
)

// awaitKinds are the only human gates an unattended run may stop at.
var awaitKinds = []string{"spec", "plan", "data-write", "irreversible", "needs-human", "question"}

// awaitRecord is what the worker host picks up from $ZENIFY_HOME/await/.
type awaitRecord struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary"`
	Files     []string  `json:"files"`
	Command   string    `json:"command"`
	Task      string    `json:"task"`
	Repo      string    `json:"repo"`
	Branch    string    `json:"branch"`
	Cwd       string    `json:"cwd"`
	CreatedAt time.Time `json:"created_at"`
	// Questions is set only for kind "question"; zenie reads it.
	Questions []awaitQuestion `json:"questions,omitempty"`
}

// awaitQuestion is one structured question put to people. Options is empty for
// a free-text answer; Audience is "business" or "technical".
type awaitQuestion struct {
	Text     string   `json:"text"`
	Options  []string `json:"options"`
	Audience string   `json:"audience"`
}

const (
	maxAwaitQuestions = 5
	maxAwaitOptions   = 4
)

// loadAwaitQuestions reads and validates the --questions file.
func loadAwaitQuestions(path string) ([]awaitQuestion, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- path is the caller's own --questions flag
	if err != nil {
		return nil, fmt.Errorf("--questions %s: %w", path, err)
	}
	var qs []awaitQuestion
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&qs); err != nil {
		return nil, fmt.Errorf("--questions %s: want a JSON array of {text, options, audience}: %w", path, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("--questions %s: unexpected data after the JSON array", path)
	}
	if len(qs) < 1 || len(qs) > maxAwaitQuestions {
		return nil, fmt.Errorf("--questions: want 1-%d questions, got %d", maxAwaitQuestions, len(qs))
	}
	for i := range qs {
		q := &qs[i]
		if strings.TrimSpace(q.Text) == "" {
			return nil, fmt.Errorf("--questions: question %d has empty text", i+1)
		}
		if len(q.Options) > maxAwaitOptions {
			return nil, fmt.Errorf("--questions: question %d has %d options, want at most %d", i+1, len(q.Options), maxAwaitOptions)
		}
		for _, o := range q.Options {
			if strings.TrimSpace(o) == "" {
				return nil, fmt.Errorf("--questions: question %d has an empty option", i+1)
			}
		}
		if q.Options == nil {
			q.Options = []string{}
		}
		if q.Audience != "business" && q.Audience != "technical" {
			return nil, fmt.Errorf("--questions: question %d audience %q, want business or technical", i+1, q.Audience)
		}
	}
	return qs, nil
}

func awaitDir() string {
	return filepath.Join(zenifyHome(os.Getenv, os.UserHomeDir), "await")
}

func newAwaitCmd() *cobra.Command {
	var summary, command, questions string
	var files []string
	cmd := &cobra.Command{
		Use:   "await <kind>",
		Short: "Dừng chạy không người ở một cổng cần người duyệt (thoát mã 8)", //znf:allow-lang
		Long:  "Các kind hợp lệ: " + strings.Join(awaitKinds, ", "),            //znf:allow-lang
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := awaitNow(args[0], summary, command, questions, files)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), id)
			return exitcode.New(exitcode.Awaiting, fmt.Errorf("awaiting human: %s (%s)", args[0], id))
		},
	}
	cmd.Flags().StringVar(&summary, "summary", "", "tóm tắt điều cần người quyết định")                                 //znf:allow-lang
	cmd.Flags().StringArrayVar(&files, "file", nil, "file người cần xem (lặp được)")                                    //znf:allow-lang
	cmd.Flags().StringVar(&command, "command", "", "lệnh người cần chạy (data-write)")                                  //znf:allow-lang
	cmd.Flags().StringVar(&questions, "questions", "", "file JSON các câu hỏi: [{text, options, audience}] (question)") //znf:allow-lang
	cmd.AddCommand(newAwaitShowCmd())
	return cmd
}

func newAwaitShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "In bản ghi await theo id", //znf:allow-lang
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if id == "" || filepath.Base(id) != id || strings.HasPrefix(id, ".") {
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("invalid await id %q", id))
			}
			b, err := os.ReadFile(filepath.Join(awaitDir(), id+".json")) //nolint:gosec // G304 -- id is validated as a bare file name above
			if err != nil {
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("no await record %q", id))
			}
			_, _ = cmd.OutOrStdout().Write(b)
			return nil
		},
	}
}

// awaitNow builds and writes one await record and returns its id. An invalid
// request (not autonomous, unknown kind, empty summary, …) is BadArgs.
func awaitNow(kind, summary, command, questionsPath string, files []string) (string, error) {
	rec, err := buildAwaitRecord(kind, summary, command, questionsPath, files)
	if err != nil {
		return "", exitcode.New(exitcode.BadArgs, err)
	}
	if err := writeAwaitRecord(rec); err != nil {
		return "", err
	}
	return rec.ID, nil
}

func buildAwaitRecord(kind, summary, command, questionsPath string, files []string) (awaitRecord, error) {
	if os.Getenv("ZNF_AUTONOMOUS") != "1" {
		return awaitRecord{}, fmt.Errorf("zenify await only works in autonomous mode (ZNF_AUTONOMOUS=1)")
	}
	valid := false
	for _, k := range awaitKinds {
		valid = valid || k == kind
	}
	if !valid {
		return awaitRecord{}, fmt.Errorf("unknown kind %q, want one of: %s", kind, strings.Join(awaitKinds, ", "))
	}
	if strings.TrimSpace(summary) == "" {
		return awaitRecord{}, fmt.Errorf("--summary is required: what the person must decide")
	}
	if kind == "data-write" && !strings.Contains(command, "zenify data run --prd --write") {
		return awaitRecord{}, fmt.Errorf("data-write needs --command with the `zenify data run --prd --write` command a lead runs")
	}
	var questions []awaitQuestion
	switch {
	case kind == "question":
		if questionsPath == "" {
			return awaitRecord{}, fmt.Errorf("question needs --questions <file> with a JSON array of {text, options, audience}")
		}
		qs, err := loadAwaitQuestions(questionsPath)
		if err != nil {
			return awaitRecord{}, err
		}
		questions = qs
	case questionsPath != "":
		return awaitRecord{}, fmt.Errorf("--questions is only for kind question")
	}
	abs := make([]string, 0, len(files))
	for _, f := range files {
		a, err := filepath.Abs(f)
		if err != nil {
			return awaitRecord{}, err
		}
		if _, err := os.Stat(a); err != nil {
			return awaitRecord{}, fmt.Errorf("--file %s: %w", f, err)
		}
		abs = append(abs, a)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return awaitRecord{}, err
	}
	var rnd [4]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return awaitRecord{}, err
	}
	now := time.Now().UTC()
	rec := awaitRecord{
		ID:        fmt.Sprintf("%s-%s-%s", kind, now.Format("20060102T150405Z"), hex.EncodeToString(rnd[:])),
		Kind:      kind,
		Summary:   summary,
		Files:     abs,
		Command:   command,
		Cwd:       cwd,
		CreatedAt: now,
		Questions: questions,
	}
	if task, ok := brokerclient.TaskHeader(); ok {
		rec.Task = task
	}
	// Outside a git repo (or on an unborn branch) these stay empty.
	r := gitx.ExecRunner()
	if b, err := r.Run(cwd, "rev-parse", "--show-toplevel"); err == nil {
		rec.Repo = strings.TrimSpace(string(b))
	}
	if b, err := r.Run(cwd, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		rec.Branch = strings.TrimSpace(string(b))
	}
	return rec, nil
}

func writeAwaitRecord(rec awaitRecord) error {
	dir := awaitDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	// Written to .<id>.json.tmp then renamed, so the host never reads half a record.
	tmp := filepath.Join(dir, "."+rec.ID+".json.tmp")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304 -- rec.ID is generated by buildAwaitRecord
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, rec.ID+".json")); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
