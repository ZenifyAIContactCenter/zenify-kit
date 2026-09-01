// Package exitcode defines the CLI's meaningful exit codes (FR-053) and an
// error type that carries one, so main can translate errors to process codes.
package exitcode

import "errors"

const (
	OK        = 0
	Fail      = 1
	BadArgs   = 2
	Cancelled = 3
	LockHeld  = 4
)

// Error carries an exit code alongside an underlying error.
type Error struct {
	Code int
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// New wraps err with an exit code.
func New(code int, err error) *Error { return &Error{Code: code, Err: err} }

// Code extracts the exit code from err: nil→OK, an *Error→its code, else Fail.
func Code(err error) int {
	if err == nil {
		return OK
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return Fail
}
