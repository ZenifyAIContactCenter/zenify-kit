package wt

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// SweepAllOptions drives RunSweepAll. FetchRunner is bounded (TimeoutRunner);
// Runner is the ordinary runner for the local sweep. Repos overrides
// enumeration (tests); nil means WorkspaceRepos(WorkspaceRoot).
type SweepAllOptions struct {
	WorkspaceRoot string
	Host          string
	DryRun        bool
	Pid           int
	Now           int64
	Runner        gitx.Runner
	FetchRunner   gitx.Runner
	Repos         []string
	Stdout        io.Writer
	Stderr        io.Writer
}

// RunSweepAll sweeps every wt-managed repo in the workspace: fetch all repos in
// parallel (each bounded by FetchRunner; a failed or timed-out fetch only warns
// — the sweep then reads local refs, which can under-report merged work but
// never over-report it), then sweep repos one at a time (rm is local git + lsof;
// serial avoids lock contention and lsof storms). A repo that errors is
// skipped, never fatal. Also removes the legacy global index file once.
func RunSweepAll(o SweepAllOptions) error {
	if o.Stdout == nil {
		o.Stdout = io.Discard
	}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	repos := o.Repos
	if repos == nil {
		repos = WorkspaceRepos(o.WorkspaceRoot)
	}
	if o.DryRun {
		if p, ok := legacyIndexPath(); ok {
			_, _ = fmt.Fprintf(o.Stdout, "wt: would remove legacy wt-index.json (%s)\n", p)
		}
	} else {
		removeLegacyIndex(o.Stdout)
	}

	// Fetch every repo in parallel, bounded by FetchRunner — dry-run also
	// fetches, since the report is only as good as the refs it reads.
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, repo := range repos {
		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			if _, err := o.FetchRunner.Run(repo, "fetch", "origin", "--quiet"); err != nil {
				mu.Lock()
				_, _ = fmt.Fprintf(o.Stderr, "wt: %s: fetch failed (%v) — merge state may be stale\n", filepath.Base(repo), err)
				mu.Unlock()
			}
		}(repo)
	}
	wg.Wait()

	swept, left, ok := 0, 0, 0
	for _, repo := range repos {
		_, _ = fmt.Fprintf(o.Stdout, "== %s ==\n", filepath.Base(repo))
		var buf countingWriter
		err := RunSweep(SweepOptions{RepoRoot: repo, Host: o.Host, DryRun: o.DryRun, Pid: o.Pid, Now: o.Now,
			Runner: o.Runner, Stdout: io.MultiWriter(o.Stdout, &buf), Stderr: o.Stderr})
		if err != nil {
			_, _ = fmt.Fprintf(o.Stderr, "wt: %s: skipped (%v)\n", filepath.Base(repo), err)
			continue
		}
		s, l := buf.totals()
		swept += s
		left += l
		ok++
	}
	if o.DryRun {
		_, _ = fmt.Fprintf(o.Stdout, "wt: %d would be removed, %d left alone across %d repos\n", swept, left, ok)
	} else {
		_, _ = fmt.Fprintf(o.Stdout, "wt: swept %d, left %d across %d repos\n", swept, left, ok)
	}
	return nil
}

// countingWriter scans RunSweep's stdout for its summary line so RunSweepAll can
// total across repos without changing RunSweep's signature.
type countingWriter struct{ swept, left int }

func (c *countingWriter) Write(p []byte) (int, error) {
	var s, l int
	if _, err := fmt.Sscanf(string(p), "wt: swept %d, left %d", &s, &l); err == nil {
		c.swept, c.left = s, l
	} else if _, err := fmt.Sscanf(string(p), "wt: %d would be removed, %d left alone", &s, &l); err == nil {
		c.swept, c.left = s, l
	}
	return len(p), nil
}

func (c *countingWriter) totals() (int, int) { return c.swept, c.left }

// legacyIndexPath is where builds before this one wrote the (never read)
// global index. Kept only so RunSweepAll can delete it.
func legacyIndexPath() (string, bool) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		base = filepath.Join(home, ".local", "state")
	}
	p := filepath.Join(base, "zenify", "wt-index.json")
	if _, err := os.Stat(p); err != nil { //nolint:gosec // G703 -- p is built from XDG_STATE_HOME or the user home plus fixed segments, and is only probed for the legacy index file
		return "", false
	}
	return p, true
}

func removeLegacyIndex(out io.Writer) {
	if p, ok := legacyIndexPath(); ok {
		if err := os.Remove(p); err == nil {
			_, _ = fmt.Fprintln(out, "wt: removed legacy wt-index.json")
		}
	}
}
