// Package perlogin caps how many requests one login runs at once.
package perlogin

import "sync"

// Slots counts in-flight requests per login. The zero value is ready to use.
type Slots struct {
	mu     sync.Mutex
	active map[string]int
}

// Acquire takes a slot for login unless it already holds limit; limit 0 refuses
// every request. A true return must be paired with Release.
func (s *Slots) Acquire(login string, limit int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil {
		s.active = map[string]int{}
	}
	if s.active[login] >= limit {
		return false
	}
	s.active[login]++
	return true
}

// Release gives back a slot taken by Acquire.
func (s *Slots) Release(login string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[login]--; s.active[login] <= 0 {
		delete(s.active, login)
	}
}
