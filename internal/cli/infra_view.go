package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/infra"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
)

// stepOutcome is how a step ended: detail is shown after its label; skipped
// marks a step that had nothing to do.
type stepOutcome struct {
	detail  string
	skipped bool
}

// stepView renders `infra up`/`down` as a numbered step list on a ui.Flow. On
// a TTY the running step has a spinner with its current sub-action, elapsed
// time and the last build log line; otherwise each state change is one plain
// line. Every line, and every build log line, also goes to the run log.
// It implements infra.Progress for the step that is running.
type stepView struct {
	flow    *ui.Flow
	out     io.Writer
	styled  bool
	resume  string // command that resumes after an interrupt
	steps   []string
	verbose bool
	log     io.Writer
	logPath string

	printMu sync.Mutex // orders spinner stop/start with printed lines
	sp      *ui.Spinner

	mu       sync.Mutex // state the spinner label reads
	cur      int
	started  time.Time
	sub      string
	subStart time.Time
	tail     string
}

// tailWidth bounds the build log line shown in the spinner label.
const tailWidth = 60

// openRunLog creates ~/.zenify/infra/logs/<cmd>-<time>.log.
func openRunLog(home, cmd string) (*os.File, string, error) {
	p := infra.LogPath(home, cmd+"-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, "", fmt.Errorf("run log: %w", err)
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // G304 -- path built by the kit under its own infra dir
	if err != nil {
		return nil, "", fmt.Errorf("run log: %w", err)
	}
	return f, p, nil
}

// newStepView prints the flow title, the log path and every step as pending.
func newStepView(out io.Writer, title, resume string, steps []string, verbose bool, log io.Writer, logPath string) *stepView {
	u := ui.New(out)
	v := &stepView{flow: u.Flow(title), out: out, styled: u.Styled(), resume: resume, steps: steps,
		verbose: verbose, log: log, logPath: logPath, cur: -1}
	if logPath != "" {
		v.flow.Note("log: " + logPath)
	}
	for i := range steps {
		v.flow.Line(ui.StatusInfo, v.label(i), "pending")
	}
	v.flow.Blank()
	v.logf("plan: %s", strings.Join(steps, ", "))
	return v
}

// addSteps inserts the steps in names that the plan lacks just before step
// before, and prints them as pending (later labels carry the new total).
func (v *stepView) addSteps(before string, names []string) {
	var add []string
	for _, n := range names {
		if !slices.Contains(v.steps, n) {
			add = append(add, n)
		}
	}
	if len(add) == 0 {
		return
	}
	at := slices.Index(v.steps, before)
	v.printMu.Lock()
	v.steps = slices.Insert(v.steps, at, add...)
	for _, n := range add {
		v.flow.Line(ui.StatusInfo, v.label(slices.Index(v.steps, n)), "pending")
	}
	v.printMu.Unlock()
	v.logf("plan: + %s", strings.Join(add, ", "))
}

// printLines writes plain lines under the flow and into the run log.
func (v *stepView) printLines(lines []string) {
	v.print(func() {
		for _, l := range lines {
			v.flow.Note(l)
			v.logf("%s", l)
		}
	})
}

func (v *stepView) label(i int) string {
	return fmt.Sprintf("%d/%d %s", i+1, len(v.steps), v.steps[i])
}

func (v *stepView) logf(format string, args ...any) {
	if v.log != nil {
		fmt.Fprintf(v.log, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
	}
}

// print writes lines with the spinner paused, so a line never lands mid-frame.
func (v *stepView) print(fn func()) {
	v.printMu.Lock()
	defer v.printMu.Unlock()
	if v.sp != nil {
		v.sp.Stop()
	}
	fn()
	if v.sp != nil {
		v.sp.Start()
	}
}

// spinnerLabel is "<n>/<N> <step>  <sub> <elapsed>  <build log tail>".
func (v *stepView) spinnerLabel() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cur < 0 {
		return ""
	}
	s := v.label(v.cur)
	since := v.started
	if v.sub != "" {
		s += "  " + v.sub
		since = v.subStart
	}
	s += " " + time.Since(since).Round(time.Second).String()
	if v.tail != "" {
		t := []rune(v.tail)
		if len(t) > tailWidth {
			t = append([]rune("..."), t[len(t)-tailWidth+3:]...)
		}
		s += "  " + string(t)
	}
	return s
}

// Sub implements infra.Progress: the running step's new sub-action.
func (v *stepView) Sub(text string) {
	v.mu.Lock()
	if v.cur < 0 || text == v.sub {
		v.mu.Unlock()
		return
	}
	v.sub, v.subStart, v.tail = text, time.Now(), ""
	label := v.label(v.cur)
	v.mu.Unlock()
	v.logf("%s: %s", label, text)
	if !v.styled {
		v.print(func() { v.flow.Line(ui.StatusActive, label, text) })
	}
}

// Output implements infra.Progress: one (redacted) line of a build log. It
// goes to the run log; the screen gets it only as the spinner tail, or in
// full with --verbose.
func (v *stepView) Output(line string) {
	if v.log != nil {
		fmt.Fprintln(v.log, line)
	}
	if strings.TrimSpace(line) == "" {
		return
	}
	v.mu.Lock()
	v.tail = strings.TrimSpace(line)
	v.mu.Unlock()
	if v.verbose {
		v.print(func() { v.flow.Note(line) })
	}
}

// run runs step name: running, then done (with its duration), skipped or
// failed. A failure after Ctrl-C names the step and how to resume.
func (v *stepView) run(ctx context.Context, name string, fn func(ctx context.Context) (stepOutcome, error)) error {
	i := -1
	for j, s := range v.steps {
		if s == name {
			i = j
		}
	}
	if i < 0 {
		return failStep(name, fmt.Errorf("step %q is not in the plan", name))
	}
	v.mu.Lock()
	v.cur, v.started, v.sub, v.tail = i, time.Now(), "", ""
	v.mu.Unlock()
	label := v.label(i)
	v.logf("%s: running", label)
	v.printMu.Lock()
	if v.styled {
		v.sp = ui.NewSpinnerFunc(v.out, v.spinnerLabel)
		v.sp.Start()
	} else {
		v.flow.Line(ui.StatusActive, label, "running")
	}
	v.printMu.Unlock()

	res, err := fn(infra.WithProgress(ctx, v))

	v.printMu.Lock()
	defer v.printMu.Unlock()
	if v.sp != nil {
		v.sp.Stop()
		v.sp = nil
	}
	v.mu.Lock()
	dur := time.Since(v.started).Round(time.Second)
	v.cur = -1
	v.mu.Unlock()
	if err != nil {
		first, rest, _ := strings.Cut(err.Error(), "\n")
		v.flow.Line(ui.StatusFail, label, first)
		for _, l := range strings.Split(rest, "\n") {
			if strings.TrimSpace(l) != "" {
				v.flow.Note(l)
			}
		}
		v.logf("%s: failed after %s: %v", label, dur, err)
		if ctx.Err() != nil {
			v.flow.Note(fmt.Sprintf("interrupted at step %s; rerun `%s` to resume", label, v.resume))
		}
		if v.logPath != "" {
			v.flow.Note("full log: " + v.logPath)
		}
		return failStep(name, err)
	}
	status := ui.StatusOK
	if res.skipped {
		status = ui.StatusInfo
	}
	detail := strings.TrimSpace(res.detail + " " + dur.String())
	v.flow.Line(status, label, detail)
	v.logf("%s: %s", label, detail)
	return nil
}

// close ends the flow with summary.
func (v *stepView) close(summary string) {
	v.flow.Close(summary)
	v.logf("%s", summary)
}
