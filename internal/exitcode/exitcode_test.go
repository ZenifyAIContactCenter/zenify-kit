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
