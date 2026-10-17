// Package datarun computes the command key of a `zenify data run` and stores
// dry-run records, so a --write run can require a matching, recent, single-use
// dry-run (FR-4). It never talks to the broker.
package datarun

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Window is how long a dry-run record stays valid for a --write run.
const Window = 60 * time.Minute

// ErrNoDryRun means no usable dry-run record matches the write.
var ErrNoDryRun = errors.New("no matching dry-run record")

// Key hashes argv (minus tokens exactly "--dry-run"), cwd, and the contents of
// every remaining argv that names an existing regular file. Fields are
// length-prefixed, so token boundaries are unambiguous.
func Key(argv []string, cwd string, readFile func(string) ([]byte, error)) (string, error) {
	h := sha256.New()
	put := func(tag byte, b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		h.Write([]byte{tag})
		h.Write(n[:])
		h.Write(b)
	}
	var files [][]byte
	for _, a := range argv {
		if a == "--dry-run" {
			continue
		}
		put('a', []byte(a))
		p := a
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			b, err := readFile(p)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", a, err)
			}
			files = append(files, b)
		}
	}
	put('c', []byte(cwd))
	for _, f := range files {
		put('f', f)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Store keeps one JSON file per dry-run under Dir.
type Store struct {
	Dir string
	Now func() time.Time
}

type record struct {
	ID    string    `json:"id"`
	Key   string    `json:"key"`
	Env   string    `json:"env"`
	Login string    `json:"login"`
	Time  time.Time `json:"time"`
	Exit  int       `json:"exit"`
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// RecordDryRun writes a new record for one dry-run execution.
func (s Store) RecordDryRun(key, env, login string, exit int) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	t := s.now()
	for i := 0; ; i++ {
		id := fmt.Sprintf("%d-%d", t.UnixNano(), i)
		r := record{ID: id, Key: key, Env: env, Login: login, Time: t, Exit: exit}
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(filepath.Join(s.Dir, id+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		_, werr := f.Write(b)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		return werr
	}
}

func (s Store) load(path string) (record, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return record{}, false
	}
	var r record
	if json.Unmarshal(b, &r) != nil || r.ID == "" {
		return record{}, false
	}
	return r, true
}

// FindForWrite returns the id of the newest unused exit-0 record with the same
// key and env within Window, else ErrNoDryRun.
func (s Store) FindForWrite(key, env string) (string, error) {
	ents, err := os.ReadDir(s.Dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNoDryRun
		}
		return "", err
	}
	now := s.now()
	var best record
	found := false
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		r, ok := s.load(filepath.Join(s.Dir, e.Name()))
		if !ok || r.Exit != 0 || r.Key != key || r.Env != env || now.Sub(r.Time) > Window {
			continue
		}
		if !found || r.Time.After(best.Time) {
			best, found = r, true
		}
	}
	if !found {
		return "", ErrNoDryRun
	}
	return best.ID, nil
}

// MarkUsed atomically claims a record by renaming it to <id>.used, which
// FindForWrite ignores. A caller that loses the race gets ErrNoDryRun.
func (s Store) MarkUsed(id string) error {
	if id == "" || id != filepath.Base(id) || strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("invalid record id %q", id)
	}
	err := os.Rename(filepath.Join(s.Dir, id+".json"), filepath.Join(s.Dir, id+".used"))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNoDryRun
	}
	return err
}
