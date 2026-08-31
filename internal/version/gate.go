package version

import (
	"errors"
	"fmt"

	"golang.org/x/mod/semver"
)

// ErrTooOld signals the binary is older than a managed repo's declared minimum.
var ErrTooOld = errors.New("zenify binary is too old")

// MeetsMin reports whether current >= min (semver). A "dev" build is never blocked.
func MeetsMin(current, min string) (bool, error) {
	if current == "dev" {
		return true, nil
	}
	if !semver.IsValid(current) {
		return false, fmt.Errorf("invalid current version %q", current)
	}
	if !semver.IsValid(min) {
		return false, fmt.Errorf("invalid min version %q", min)
	}
	return semver.Compare(current, min) >= 0, nil
}

// GuardMutation returns ErrTooOld (with an upgrade hint) when current < min.
// Every mutating command must call this before writing anything.
func GuardMutation(current, min string) error {
	ok, err := MeetsMin(current, min)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: have %s, this repo requires >= %s — upgrade zenify first",
			ErrTooOld, current, min)
	}
	return nil
}
