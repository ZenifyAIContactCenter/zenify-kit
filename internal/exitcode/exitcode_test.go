package exitcode

import (
	"errors"
	"testing"
)

func TestCode(t *testing.T) {
	if Code(nil) != OK {
		t.Errorf("nil → %d, want OK", Code(nil))
	}
	if Code(errors.New("plain")) != Fail {
		t.Errorf("plain error → %d, want Fail", Code(errors.New("plain")))
	}
	e := New(LockHeld, errors.New("held"))
	if Code(e) != LockHeld {
		t.Errorf("Error → %d, want LockHeld", Code(e))
	}
	if !errors.Is(e, e.Err) {
		t.Errorf("Unwrap broken")
	}
	wrapped := errors.New("outer")
	_ = wrapped
}

func TestAwaitingCode(t *testing.T) {
	if Awaiting != 8 {
		t.Errorf("Awaiting = %d, want 8", Awaiting)
	}
	if Code(New(Awaiting, errors.New("w"))) != 8 {
		t.Errorf("Code(Awaiting error) != 8")
	}
}
