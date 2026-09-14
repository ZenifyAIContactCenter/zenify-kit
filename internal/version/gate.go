package version

import (
	"errors"
	"fmt"

	"golang.org/x/mod/semver"
)

// ErrTooOld signals the binary is older than a managed repo's declared minimum.
var ErrTooOld = errors.New("zenify binary is too old")

// canonical returns v with the "v" prefix golang.org/x/mod/semver requires.
// goreleaser injects {{.Version}} WITHOUT the prefix ("0.17.2"), while the
// floors in code are written "v0.3.0" — accept both spellings.
func canonical(v string) string {
	if v == "" || v[0] == 'v' {
		return v
	}
	return "v" + v
}

// MeetsMin reports whether current >= min (semver). A "dev" build is never blocked.
// Both arguments may be given with or without the leading "v".
func MeetsMin(current, min string) (bool, error) {
	if current == "dev" {
		return true, nil
	}
	current, min = canonical(current), canonical(min)
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

// Newer reports whether latest is strictly greater than current (semver).
// A "dev" build is never behind; either side failing semver parsing yields
// false, so a garbled release tag can never produce a nudge.
func Newer(latest, current string) bool {
	if current == "dev" {
		return false
	}
	latest, current = canonical(latest), canonical(current)
	if !semver.IsValid(latest) || !semver.IsValid(current) {
		return false
	}
	return semver.Compare(latest, current) > 0
}
