package infra

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// Progress receives what a long step is doing now. Sub is a new sub-action
// ("waiting node Ready"); Output is one line of a child's output (a build log),
// already redacted.
type Progress interface {
	Sub(text string)
	Output(line string)
}

type progressKey struct{}

// WithProgress makes the waits and builds under ctx report to p.
func WithProgress(ctx context.Context, p Progress) context.Context {
	return context.WithValue(ctx, progressKey{}, p)
}

func progressFrom(ctx context.Context) Progress {
	p, _ := ctx.Value(progressKey{}).(Progress)
	return p
}

// report tells the step's view what is happening now; no-op without one.
func report(ctx context.Context, format string, args ...any) {
	if p := progressFrom(ctx); p != nil {
		p.Sub(fmt.Sprintf(format, args...))
	}
}

// LogPath is where a run of `infra up`/`down` keeps its full log.
func LogPath(home, run string) string { return filepath.Join(infraDir(home), "logs", run+".log") }

// lineWriter hands each complete line to p.Output with every secret value
// replaced; a value never spans lines, so per-line redaction is complete.
type lineWriter struct {
	p       Progress
	secrets []string
	buf     []byte
}

func (l *lineWriter) Write(b []byte) (int, error) {
	l.buf = append(l.buf, b...)
	for {
		i := bytes.IndexByte(l.buf, '\n')
		if i < 0 {
			return len(b), nil
		}
		l.emit(string(l.buf[:i]))
		l.buf = l.buf[i+1:]
	}
}

func (l *lineWriter) emit(line string) {
	line = strings.TrimRight(line, "\r")
	for _, v := range l.secrets {
		line = strings.ReplaceAll(line, v, "[redacted]")
	}
	l.p.Output(line)
}

// flush emits a last line that had no newline.
func (l *lineWriter) flush() {
	if len(l.buf) > 0 {
		l.emit(string(l.buf))
		l.buf = nil
	}
}
