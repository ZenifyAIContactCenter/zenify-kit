// Package audit writes one JSON line per broker request. Entries never carry
// result content, passwords, JWTs or GitHub tokens; callers fill Args only
// with request parameters.
package audit

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// Entry is one audit line, keyed as in the spec's Contract section.
type Entry struct {
	TS       time.Time `json:"ts"`
	Login    string    `json:"login"`
	Role     string    `json:"role"`
	Endpoint string    `json:"endpoint"`
	Env      string    `json:"env"`
	Op       string    `json:"op"`
	Args     any       `json:"args"`
	Rows     int       `json:"rows"`
	Bytes    int       `json:"bytes"`
	MS       int64     `json:"ms"`
	Status   int       `json:"status"`
}

// Logger serialises entries to W, one line each.
type Logger struct {
	W  io.Writer
	mu sync.Mutex
}

// Log writes e as a single JSON line. A failed write is dropped: auditing must
// not take the request down.
func (l *Logger) Log(e Entry) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.W.Write(append(b, '\n'))
}
