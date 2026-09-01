package lock

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeLock(t *testing.T, dir string, info Info) {
	t.Helper()
	b, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, lockName), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAcquire_FreshSucceeds(t *testing.T) {
	dir := t.TempDir()
	h, err := Acquire(dir, os.Getpid(), "host-a", 1000)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if h == nil {
		t.Fatal("nil handle")
	}
	if _, err := os.Stat(filepath.Join(dir, lockName)); err != nil {
		t.Errorf("lockfile missing: %v", err)
	}
}

func TestAcquire_HeldByLiveProcessFails(t *testing.T) {
	dir := t.TempDir()
	// PID 1 (init) is always alive and is not our PID.
	writeLock(t, dir, Info{PID: 1, Host: "other", CreatedAt: 1})
	_, err := Acquire(dir, os.Getpid(), "host-a", 2000)
	if !errors.Is(err, ErrHeld) {
		t.Errorf("err = %v, want ErrHeld", err)
	}
}

func TestAcquire_StaleLockReplaced(t *testing.T) {
	dir := t.TempDir()
	// A PID that is not alive (very large, unlikely to exist).
	writeLock(t, dir, Info{PID: 1 << 30, Host: "dead", CreatedAt: 1})
	h, err := Acquire(dir, os.Getpid(), "host-a", 3000)
	if err != nil {
		t.Fatalf("acquire over stale lock: %v", err)
	}
	if h == nil {
		t.Fatal("nil handle")
	}
	got, err := readInfo(filepath.Join(dir, lockName))
	if err != nil {
		t.Fatal(err)
	}
	if got.PID != os.Getpid() {
		t.Errorf("lock PID = %d, want %d", got.PID, os.Getpid())
	}
}

func TestAcquire_OwnLeftoverReplaced(t *testing.T) {
	dir := t.TempDir()
	writeLock(t, dir, Info{PID: os.Getpid(), Host: "host-a", CreatedAt: 1})
	h, err := Acquire(dir, os.Getpid(), "host-a", 4000)
	if err != nil {
		t.Fatalf("acquire over own leftover: %v", err)
	}
	if h == nil {
		t.Fatal("nil handle")
	}
}

func TestAcquire_CorruptLockReplaced(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, lockName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := Acquire(dir, os.Getpid(), "host-a", 5000)
	if err != nil {
		t.Fatalf("acquire over corrupt lock: %v", err)
	}
	if h == nil {
		t.Fatal("nil handle")
	}
}

func TestRelease_FreesLock(t *testing.T) {
	dir := t.TempDir()
	h, err := Acquire(dir, os.Getpid(), "host-a", 6000)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, lockName)); !os.IsNotExist(err) {
		t.Errorf("lockfile still present after release: %v", err)
	}
	// Re-acquire must now succeed.
	if _, err := Acquire(dir, os.Getpid(), "host-a", 7000); err != nil {
		t.Errorf("re-acquire after release: %v", err)
	}
}
