package devenv

import (
	"bytes"
	"io"
	"sort"
	"strings"
)

const minMaskLen = 6

// MaskWriter replaces secret values with *** in line-buffered output.
type MaskWriter struct {
	w   io.Writer
	r   *strings.Replacer
	buf []byte
}

// NewMaskWriter masks secrets of at least 6 characters. Longer secrets are
// replaced first so a shorter one cannot expose the rest of a longer one.
func NewMaskWriter(w io.Writer, secrets []string) *MaskWriter {
	var keep []string
	for _, s := range secrets {
		if len(s) >= minMaskLen {
			keep = append(keep, s)
		}
	}
	sort.SliceStable(keep, func(i, j int) bool { return len(keep[i]) > len(keep[j]) })
	pairs := make([]string, 0, len(keep)*2)
	for _, s := range keep {
		pairs = append(pairs, s, "***")
	}
	return &MaskWriter{w: w, r: strings.NewReplacer(pairs...)}
}

// Write buffers until a full line is available, then writes it masked.
func (m *MaskWriter) Write(p []byte) (int, error) {
	m.buf = append(m.buf, p...)
	for {
		i := bytes.IndexByte(m.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		line := m.buf[:i+1]
		if _, err := io.WriteString(m.w, m.r.Replace(string(line))); err != nil {
			return 0, err
		}
		m.buf = m.buf[i+1:]
	}
}

// Close flushes a trailing partial line.
func (m *MaskWriter) Close() error {
	if len(m.buf) == 0 {
		return nil
	}
	_, err := io.WriteString(m.w, m.r.Replace(string(m.buf)))
	m.buf = nil
	return err
}
