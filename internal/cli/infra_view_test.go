package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Piped/CI output must stay one plain line per state change (no spinner, no
// ANSI), with the build log kept out of the screen but in the run log, so a
// dev sees which step is running and why it failed without a log flood.
func TestStepView_PlainLinesPerStateChange(t *testing.T) {
	var out, log bytes.Buffer
	v := newStepView(&out, "zenify infra up", "zenify infra up --local", []string{"k3s", "build shop-api", "env-phase-1", "state"}, false, &log, "/h/run.log")
	ctx := context.Background()
	if err := v.run(ctx, "k3s", func(ctx context.Context) (stepOutcome, error) {
		v.Sub("waiting node Ready")
		v.Sub("waiting node Ready") // same sub-action: no new line
		return stepOutcome{detail: "changed"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := v.run(ctx, "build shop-api", func(ctx context.Context) (stepOutcome, error) {
		v.Output("#5 [2/9] RUN npm ci")
		return stepOutcome{detail: "sha-1 unchanged (tag exists)", skipped: true}, nil
	}); err != nil {
		t.Fatal(err)
	}
	err := v.run(ctx, "env-phase-1", func(ctx context.Context) (stepOutcome, error) {
		return stepOutcome{}, errors.New("wait app env-base: not ready\n  Deployment be: Degraded")
	})
	if err == nil || !strings.HasPrefix(err.Error(), "env-phase-1: ") {
		t.Fatalf("err = %v", err)
	}
	got := out.String()
	if strings.Contains(got, "\x1b[") || strings.Contains(got, "\r") {
		t.Fatalf("plain output has ANSI or frames: %q", got)
	}
	want := []string{
		"│  log: /h/run.log",
		"│  • 1/4 k3s   pending",
		"│  • 4/4 state   pending",
		"│  › 1/4 k3s   running",
		"│  › 1/4 k3s   waiting node Ready",
		"│  ✓ 1/4 k3s   changed 0s",
		"│  › 2/4 build shop-api   running",
		"│  • 2/4 build shop-api   sha-1 unchanged (tag exists) 0s",
		"│  › 3/4 env-phase-1   running",
		"│  ✗ 3/4 env-phase-1   wait app env-base: not ready",
		"│    Deployment be: Degraded",
		"│  full log: /h/run.log",
	}
	lines := strings.Split(got, "\n")
	at := 0
	for _, w := range want {
		for at < len(lines) && lines[at] != w {
			at++
		}
		if at == len(lines) {
			t.Fatalf("missing or out of order %q in:\n%s", w, got)
		}
	}
	if strings.Count(got, "waiting node Ready") != 1 || strings.Contains(got, "npm ci") || strings.Contains(got, "4/4 state   running") {
		t.Fatalf("repeated sub-action, build log on screen, or a step after the failure:\n%s", got)
	}
	if !strings.Contains(log.String(), "#5 [2/9] RUN npm ci") || !strings.Contains(log.String(), "3/4 env-phase-1: failed") {
		t.Fatalf("run log = %s", log.String())
	}
}

// --verbose shows every build log line; Ctrl-C names the step it stopped at
// and how to resume.
func TestStepView_VerboseAndInterrupt(t *testing.T) {
	var out bytes.Buffer
	v := newStepView(&out, "zenify infra up", "zenify infra up --local", []string{"build shop-api"}, true, nil, "")
	ctx, cancel := context.WithCancel(context.Background())
	err := v.run(ctx, "build shop-api", func(ctx context.Context) (stepOutcome, error) {
		v.Output("#5 [2/9] RUN npm ci")
		cancel()
		return stepOutcome{}, ctx.Err()
	})
	if err == nil {
		t.Fatal("want the interrupt error")
	}
	got := out.String()
	for _, w := range []string{"│  #5 [2/9] RUN npm ci", "interrupted at step 1/1 build shop-api; rerun `zenify infra up --local` to resume"} {
		if !strings.Contains(got, w) {
			t.Fatalf("missing %q in:\n%s", w, got)
		}
	}
}

// up and down keep their full log on disk and print where it is.
func TestInfraUpDown_WriteRunLog(t *testing.T) {
	d, _, _ := fakeInfra(t, true)
	home, _ := d.home()
	for _, args := range [][]string{{"up", "--local"}, {"down", "--local", "--apply"}} {
		out, _, err := runInfra(d, args...)
		if err != nil {
			t.Fatal(err)
		}
		logs, _ := filepath.Glob(filepath.Join(home, ".zenify", "infra", "logs", args[0]+"-*.log"))
		if len(logs) != 1 || !strings.Contains(out, "log: "+logs[0]) {
			t.Fatalf("%s: logs=%v out:\n%s", args[0], logs, out)
		}
		b, err := os.ReadFile(logs[0])
		if err != nil || !strings.Contains(string(b), "plan: ") {
			t.Fatalf("%s log: %v %s", args[0], err, b)
		}
	}
}

// A plan without the anchor step must not panic: the new steps go last.
func TestStepViewAddSteps_MissingAnchorAppends(t *testing.T) {
	var out bytes.Buffer
	v := newStepView(&out, "zenify infra up", "zenify infra up --local", []string{"k3s", "state"}, false, nil, "")
	v.addSteps("no-such-step", []string{"build x"})
	if got := strings.Join(v.steps, ","); got != "k3s,state,build x" {
		t.Errorf("steps = %s", got)
	}
}

// Business rule: a second up started in the same second (refused later by the
// lock) must not truncate the log of the run already in progress.
func TestOpenRunLogNeverTruncatesASameSecondLog(t *testing.T) {
	home := t.TempDir()
	f1, p1, err := openRunLog(home, "up")
	if err != nil {
		t.Fatal(err)
	}
	defer f1.Close()
	if _, err := f1.WriteString("first run\n"); err != nil {
		t.Fatal(err)
	}
	f2, p2, err := openRunLog(home, "up")
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	if p1 == p2 {
		t.Fatalf("both runs got %s", p1)
	}
	if b, err := os.ReadFile(p1); err != nil || string(b) != "first run\n" {
		t.Errorf("first log = %q, %v", b, err)
	}
}
