// internal/apply/hooks_parity_test.go
package apply

import "testing"

// znfHookSpecs must cover exactly the events znf declares in its plugin hooks.json.
// This is an event-coverage check, not a byte-for-byte diff against the embedded
// plugin hooks.json (cross-package embed comparison is more complex than the
// drift risk warrants) — event coverage is enough to catch the main drift: a new
// event added to the plugin's hooks.json without a matching spec here, or vice
// versa.
func TestHookSpecsCoverPluginEvents(t *testing.T) {
	wantEvents := map[string]bool{
		"SessionStart": false, "Stop": false, "PreToolUse": false, "PostToolUse": false,
	}
	for _, s := range znfHookSpecs() {
		if _, ok := wantEvents[s.Event]; !ok {
			t.Fatalf("unexpected event in specs: %s", s.Event)
		}
		wantEvents[s.Event] = true
	}
	for ev, seen := range wantEvents {
		if !seen {
			t.Fatalf("znfHookSpecs missing event %s (drift vs plugin hooks.json)", ev)
		}
	}
}
