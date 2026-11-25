package infra

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// State records what `zenify infra up --local` installed.
type State struct {
	Domain        string    `json:"domain"`
	K3sVersion    string    `json:"k3sVersion"`
	ArgoCDVersion string    `json:"argocdVersion"`
	CreatedAt     time.Time `json:"createdAt"`
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

var nonDomainChars = regexp.MustCompile(`[^a-z0-9-]+`)
var dashRun = regexp.MustCompile(`-{2,}`)

// DefaultDomain derives "<login>.test" from a GitHub login.
func DefaultDomain(ghLogin string) (string, error) {
	s := nonDomainChars.ReplaceAllString(strings.ToLower(ghLogin), "-")
	s = dashRun.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "", fmt.Errorf("infra: github login %q has no usable characters for a domain", ghLogin)
	}
	return s + ".test", nil
}

// Host returns the ingress host for a service in an environment slug.
func Host(svc, slug, domain string) string {
	return svc + "-" + slug + "." + domain
}
