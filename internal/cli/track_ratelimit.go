package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
)

const (
	// backoffBase is the first wait after a failed source; a rate limit starts at
	// the 1-minute window Plane documents. The wait doubles per failure up to backoffMax.
	backoffBase        = 30 * time.Second
	backoffLimitedBase = time.Minute
	backoffMax         = 15 * time.Minute
	// backoffHeaderMax caps a server-announced wait (Retry-After / X-RateLimit-Reset).
	backoffHeaderMax = time.Hour

	backlogLock = "backlog-gh.lock"
	// planeStatesTTL is long: a project's workflow states almost never change.
	planeStatesTTL  = 24 * time.Hour
	planeStatesFile = "plane-states.json"
)

// rateLimitError is a Plane HTTP 429. until is the moment the server says to
// retry (zero when it sent no usable header).
type rateLimitError struct {
	err   error
	until time.Time
}

func (e *rateLimitError) Error() string { return e.err.Error() }
func (e *rateLimitError) Unwrap() error { return e.err }

// rateLimitUntil reads Retry-After (seconds) or X-RateLimit-Reset (epoch seconds).
func rateLimitUntil(h http.Header, now time.Time) time.Time {
	if v, err := strconv.Atoi(strings.TrimSpace(h.Get("Retry-After"))); err == nil && v > 0 {
		return now.Add(time.Duration(v) * time.Second)
	}
	if v, err := strconv.ParseInt(strings.TrimSpace(h.Get("X-RateLimit-Reset")), 10, 64); err == nil && v > 0 {
		if t := time.Unix(v, 0); t.After(now) {
			return t
		}
	}
	return time.Time{}
}

// multiErr joins per-project errors into one message and still unwraps to each,
// so errors.As finds a rateLimitError inside.
type multiErr struct {
	msg  string
	errs []error
	ok   []string // identifiers of the projects fetched in full, even when they held nothing
}

func (e *multiErr) Error() string   { return e.msg }
func (e *multiErr) Unwrap() []error { return e.errs }

// keepOldPlane is fresh plus the old items of the projects that did not
// answer in full: a project that did is replaced wholesale, so what closed
// there leaves the list.
func keepOldPlane(fresh, old []trackstats.PlaneItem, err error) []trackstats.PlaneItem {
	answered := map[string]bool{}
	var m *multiErr
	if errors.As(err, &m) {
		for _, id := range m.ok {
			answered[id] = true
		}
	}
	out := append([]trackstats.PlaneItem{}, fresh...)
	for _, it := range old {
		if i := strings.LastIndex(it.Key, "-"); i < 0 || !answered[it.Key[:i]] {
			out = append(out, it)
		}
	}
	return out
}

// sourceState is what the cache remembers about the last failed refresh of one
// source: the error to keep showing and the moment refetching may resume.
type sourceState struct {
	Err     string    `json:"err,omitempty"`
	Until   time.Time `json:"until,omitempty"`
	Fails   int       `json:"fails,omitempty"`
	Limited bool      `json:"limited,omitempty"`
}

// active reports whether the source must not be refetched now. A forced
// refresh still waits out a rate limit, but may retry any other failure.
func (s sourceState) active(now time.Time, force bool) bool {
	if s.Err == "" || !now.Before(s.Until) {
		return false
	}
	return !force || s.Limited
}

// failed returns the state after one more failure of err.
func (s sourceState) failed(err error, limited bool, announced, now time.Time) sourceState {
	n := s.Fails + 1
	wait := backoffBase
	if limited {
		wait = backoffLimitedBase
	}
	for i := 1; i < n && wait < backoffMax; i++ {
		wait *= 2
	}
	wait = min(wait, backoffMax)
	until := now.Add(wait)
	if limited && announced.After(now) {
		until = now.Add(min(announced.Sub(now), backoffHeaderMax))
	}
	return sourceState{Err: err.Error(), Until: until.UTC(), Fails: n, Limited: limited}
}

func (s sourceState) asError() error {
	if s.Err == "" {
		return nil
	}
	return errors.New(s.Err)
}

// ghRateLimited spots the rate-limit wording of a failed gh search.
func ghRateLimited(err error) bool {
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "rate limit") || strings.Contains(m, "http 429")
}

// planeRateLimit reports whether err holds a 429 and the retry moment it announced.
func planeRateLimit(err error) (bool, time.Time) {
	var rl *rateLimitError
	if !errors.As(err, &rl) {
		return false, time.Time{}
	}
	until := rl.until
	var m *multiErr
	if errors.As(err, &m) {
		for _, e := range m.errs { // the latest reset announced by any project
			var r *rateLimitError
			if errors.As(e, &r) && r.until.After(until) {
				until = r.until
			}
		}
	}
	return true, until
}

// lockStale is how long a lock file may sit before a crashed holder is assumed.
const lockStale = 2 * trackGhTimeout

// tryBacklogLock takes the cross-process refresh lock; ok is false when another
// process holds it. The lock is a file created exclusively (portable to Windows)
// that holds the owner's token: release removes the file only while it is still
// ours, and a stale lock is taken by renaming it away, which one process wins.
func tryBacklogLock(dir string, now time.Time) (release func(), ok bool, err error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, false, fmt.Errorf("create %s: %w", dir, err)
	}
	path := filepath.Join(dir, backlogLock)
	token := strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // G304 -- computed from the resolved workspace
		if err == nil {
			_, werr := f.WriteString(token)
			if cerr := errors.Join(werr, f.Close()); cerr != nil {
				_ = os.Remove(path)
				return nil, false, fmt.Errorf("lock %s: %w", path, cerr)
			}
			return func() {
				if b, rerr := os.ReadFile(path); rerr == nil && string(b) == token { //nolint:gosec // G304 -- computed from the resolved workspace
					_ = os.Remove(path)
				}
			}, true, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, false, fmt.Errorf("lock %s: %w", path, err)
		}
		fi, serr := os.Stat(path)
		if serr != nil || now.Sub(fi.ModTime()) < lockStale {
			return nil, false, nil
		}
		// Rename the stale file away: of two processes that saw it, only one rename succeeds.
		aside := path + "." + token + ".stale"
		if os.Rename(path, aside) != nil {
			return nil, false, nil
		}
		if afi, aerr := os.Stat(aside); aerr == nil && now.Sub(afi.ModTime()) < lockStale {
			_ = os.Rename(aside, path) // we moved a lock another process had just retaken: put it back
			return nil, false, nil
		}
		_ = os.Remove(aside)
	}
	return nil, false, nil
}

// planeStatesCache keeps each project's open state ids for planeStatesTTL.
type planeStatesCache struct {
	mu      sync.Mutex
	path    string
	now     time.Time
	dirty   bool
	Entries map[string]planeStatesEntry `json:"entries"`
}

type planeStatesEntry struct {
	FetchedAt time.Time `json:"fetchedAt"`
	Open      []string  `json:"open"`
}

// loadPlaneStates reads the cache; a missing file is an empty cache and an
// unreadable one is an error (the caller refetches and overwrites it).
func loadPlaneStates(dir string, now time.Time) (*planeStatesCache, error) {
	c := &planeStatesCache{path: filepath.Join(dir, planeStatesFile), now: now, Entries: map[string]planeStatesEntry{}}
	b, err := os.ReadFile(c.path) //nolint:gosec // G304 -- computed from the resolved workspace
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("read %s: %w", c.path, err)
	}
	if err := json.Unmarshal(b, c); err != nil {
		c.Entries = map[string]planeStatesEntry{}
		return c, fmt.Errorf("parse %s: %w", c.path, err)
	}
	if c.Entries == nil {
		c.Entries = map[string]planeStatesEntry{}
	}
	return c, nil
}

func (c *planeStatesCache) get(project string) ([]string, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.Entries[project]
	if !ok || c.now.Sub(e.FetchedAt) >= planeStatesTTL || c.now.Before(e.FetchedAt) {
		return nil, false
	}
	return e.Open, true
}

func (c *planeStatesCache) put(project string, open []string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Entries[project] = planeStatesEntry{FetchedAt: c.now.UTC(), Open: open}
	c.dirty = true
}

// save writes the cache when a project was added; same temp-file-then-rename as the backlog cache.
func (c *planeStatesCache) save() error {
	if c == nil || !c.dirty {
		return nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("write %s: %w", c.path, err)
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o750); err != nil {
		return fmt.Errorf("write %s: %w", c.path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.path), planeStatesFile+".*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", c.path, err)
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", c.path, err)
	}
	if err := os.Rename(tmp.Name(), c.path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", c.path, err)
	}
	return nil
}
