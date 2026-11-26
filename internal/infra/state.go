package infra

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State records what `zenify infra up --local` installed.
type State struct {
	Domain        string    `json:"domain"`
	K3sVersion    string    `json:"k3sVersion"`
	ArgoCDVersion string    `json:"argocdVersion"`
	CreatedAt     time.Time `json:"createdAt"`
	// Profile is the env profile (core or full) the last up deployed; empty = core.
	Profile string `json:"profile,omitempty"`
	// ConfigRevision is the helm commit the seed app was last synced at; a rerun
	// whose seed app still runs it skips the forced sync.
	ConfigRevision string `json:"configRevision,omitempty"`
}

func statePath(home string) string { return filepath.Join(infraDir(home), "state.json") }

// LoadState returns nil, nil when no state file exists; a corrupt file is an error.
func LoadState(home string) (*State, error) {
	data, err := os.ReadFile(statePath(home)) //nolint:gosec // G304 -- path derived from home
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("infra: read state: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("infra: parse %s: %w", statePath(home), err)
	}
	return &s, nil
}

// SaveState writes the state atomically with mode 0600.
func SaveState(home string, s *State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("infra: encode state: %w", err)
	}
	return writeFileAtomic(home, "state.json", append(data, '\n'))
}

// RemoveState deletes the state file; a missing file is not an error.
func RemoveState(home string) error {
	if err := os.Remove(statePath(home)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("infra: remove state: %w", err)
	}
	return nil
}

// DefaultDomain is the env domain when neither config, state nor a deployed
// platform names one.
const DefaultDomain = "zenify.test"

// Host returns the ingress host for a service in an environment slug: the base
// env has no slug in its hosts (web.zenify.test), others do (web-x.zenify.test).
func Host(svc, slug, domain string) string {
	if slug == BaseSlug {
		return svc + "." + domain
	}
	return svc + "-" + slug + "." + domain
}

// ActiveProfile is the profile saved in s, core when there is no state or none was saved.
func (s *State) ActiveProfile() string {
	if s == nil || s.Profile == "" {
		return ProfileCore
	}
	return s.Profile
}

// ConfigRev is the saved config revision, "" when there is no state.
func (s *State) ConfigRev() string {
	if s == nil {
		return ""
	}
	return s.ConfigRevision
}
