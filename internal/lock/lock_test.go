package lock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquire_FreshDir(t *testing.T) {
	dir := t.TempDir()
	h, err := Acquire(dir, os.Getpid(), "host-a", 1000)
	if err != nil {
		t.Fatalf("Acquire on fresh dir: %v", err)
	}
	if h == nil {
		t.Fatal("Acquire returned nil handle")
	}
	if _, err := os.Stat(filepath.Join(dir, lockName)); err != nil {
		t.Errorf("lockfile not created: %v", err)
	}
	if err := h.Release(); err != nil {
		t.Errorf("Release: %v", err)
	}
}

func TestAcquire_HeldByLiveHandle_ReturnsErrHeld(t *testing.T) {
	dir := t.TempDir()
	h1, err := Acquire(dir, os.Getpid(), "host-a", 1000)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer func() { _ = h1.Release() }()

	// Second acquire while the first handle still holds the flock must fail.
	_, err = Acquire(dir, os.Getpid()+1, "host-b", 2000)
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("want ErrHeld while held, got %v", err)
	}
}

func TestAcquire_AfterRelease_Succeeds(t *testing.T) {
	dir := t.TempDir()
	h1, err := Acquire(dir, os.Getpid(), "host-a", 1000)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	if err := h1.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	h2, err := Acquire(dir, os.Getpid(), "host-a", 3000)
	if err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
	_ = h2.Release()
}

func TestAcquire_WritesDiagnosticInfo(t *testing.T) {
	dir := t.TempDir()
	h, err := Acquire(dir, 4242, "diag-host", 555)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer func() { _ = h.Release() }()

	// The sidecar Info is readable (flock is advisory; it does not block reads).
	got, err := readInfo(filepath.Join(dir, lockName))
	if err != nil {
		t.Fatalf("readInfo: %v", err)
	}
	if got.PID != 4242 || got.Host != "diag-host" || got.CreatedAt != 555 {
		t.Errorf("sidecar Info = %+v, want pid=4242 host=diag-host created=555", got)
	}
}

func TestRelease_NilHandle(t *testing.T) {
	var h *Handle
	if err := h.Release(); err != nil {
		t.Errorf("nil Release: %v", err)
	}
}
