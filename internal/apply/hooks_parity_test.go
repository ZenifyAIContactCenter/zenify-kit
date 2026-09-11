// internal/apply/hooks_parity_test.go
package apply

import "testing"

// znfHookSpecs must cover exactly the four events the kit wires. Event coverage
// is enough to catch the main drift: a hook dispatch id added to hooksrun.go
// without a spec here, or vice versa.
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
			t.Fatalf("znfHookSpecs missing event %s", ev)
		}
	}
}
