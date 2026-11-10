package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// The ZEN project's states as Plane returns them (shape and names measured on
// plane.zenify.vn 2026-11-10; ids shortened).
const (
	stBacklog    = "st-backlog"
	stTodo       = "st-todo"
	stInProgress = "st-inprogress"
	stInReview   = "st-inreview"
	stOnStaging  = "st-onstaging"
	stReady      = "st-ready"
	stDone       = "st-done"
	stCancelled  = "st-cancelled"
	stTriage     = "st-triage" // Intake: Plane's triage state, absent from the states list
)

const zenStatesJSON = `{"results":[
{"id":"st-backlog","name":"Backlog","group":"backlog","is_triage":false},
{"id":"st-todo","name":"Todo","group":"unstarted","is_triage":false},
{"id":"st-inprogress","name":"In Progress","group":"started","is_triage":false},
{"id":"st-done","name":"Done","group":"completed","is_triage":false},
{"id":"st-cancelled","name":"Cancelled","group":"cancelled","is_triage":false},
{"id":"st-inreview","name":"In Review","group":"started","is_triage":false},
{"id":"st-onstaging","name":"On Staging","group":"started","is_triage":false},
{"id":"st-ready","name":"Ready for Release","group":"started","is_triage":false}],
"total_count":8,"next_page_results":false}`

type fakeWtPlane struct {
	srv      *httptest.Server
	mu       sync.Mutex
	requests []string // "METHOD path"
	patches  []map[string]any
	keys     []string // X-API-Key seen
	state    string
	assign   []string
	fail     bool
	meID     string // /users/me/ id, default u-dev
}

func newFakePlane(t *testing.T, state string, assignees ...string) *fakeWtPlane {
	t.Helper()
	f := &fakeWtPlane{state: state, assign: assignees, meID: "u-dev"}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		f.keys = append(f.keys, r.Header.Get("X-API-Key"))
		if f.fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/users/me/":
			_, _ = io.WriteString(w, `{"id":"`+f.meID+`","display_name":"dev","email":"dev@x"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/zenify/work-items/ZEN-7/":
			b, _ := json.Marshal(map[string]any{"id": "wi-7", "project": "p-zen", "sequence_id": 7, "state": f.state, "assignees": f.assign})
			_, _ = w.Write(b)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/zenify/projects/p-zen/states/":
			_, _ = io.WriteString(w, zenStatesJSON)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/workspaces/zenify/projects/p-zen/work-items/wi-7/":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.patches = append(f.patches, body)
			_, _ = io.WriteString(w, `{"id":"wi-7"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	saved := trustedBrokerManifest
	yaml := "org: o\nrepos:\n  - name: r\n    url: u\nplane:\n  url: " + f.srv.URL + "\n  workspace: zenify\n"
	trustedBrokerManifest = func() ([]byte, string) { return []byte(yaml), "test-plane" }
	t.Cleanup(func() { trustedBrokerManifest = saved })
	return f
}

func patEnv(k string) string {
	if k == planePATKey {
		return "pat-secret"
	}
	return ""
}

func runHook(t *testing.T, f *fakeWtPlane, slug string, getenv func(string) string) string {
	t.Helper()
	ws := planeWorkspace(t, "")
	return planeStartHook(getenv, ws, f.srv.Client())(slug)
}

func (f *fakeWtPlane) onlyPatch(t *testing.T) map[string]any {
	t.Helper()
	if len(f.patches) != 1 {
		t.Fatalf("patches = %v, want exactly one (requests %v)", f.patches, f.requests)
	}
	return f.patches[0]
}

func assigneesOf(t *testing.T, body map[string]any) []string {
	t.Helper()
	raw, ok := body["assignees"].([]any)
	if !ok {
		t.Fatalf("body %v has no assignees list", body)
	}
	var out []string
	for _, v := range raw {
		out = append(out, v.(string))
	}
	slices.Sort(out)
	return out
}

// Backlog, no assignee → In Progress, dev added.
func TestWtPlane_BacklogUnheld_MovesAndAssigns(t *testing.T) {
	f := newFakePlane(t, stBacklog)
	if w := runHook(t, f, "zen-7", patEnv); w != "" {
		t.Fatalf("warning %q, want none", w)
	}
	body := f.onlyPatch(t)
	if body["state"] != stInProgress {
		t.Fatalf("state = %v, want %s", body["state"], stInProgress)
	}
	if got := assigneesOf(t, body); !slices.Equal(got, []string{"u-dev"}) {
		t.Fatalf("assignees = %v, want [u-dev]", got)
	}
	for _, k := range f.keys {
		if k != "pat-secret" {
			t.Fatalf("X-API-Key = %q, want the PAT", k)
		}
	}
}

// Backlog is a cycle column now, so an assignee may hold it: same rules as Todo.
func TestStartZenItem_BacklogOtherAssignee_Warns(t *testing.T) {
	f := newFakePlane(t, stBacklog, "u-other")
	w := runHook(t, f, "zen-7", patEnv)
	if !strings.Contains(w, "held by someone else") {
		t.Fatalf("warning %q, want 'held by someone else'", w)
	}
	if len(f.patches) != 0 {
		t.Fatalf("patched %v", f.patches)
	}
}

func TestStartZenItem_BacklogDevAndOthers_Warns(t *testing.T) {
	f := newFakePlane(t, stBacklog, "u-dev", "u-other")
	w := runHook(t, f, "zen-7", patEnv)
	if !strings.Contains(w, "holder unknown") {
		t.Fatalf("warning %q, want 'holder unknown'", w)
	}
	if len(f.patches) != 0 {
		t.Fatalf("patched %v", f.patches)
	}
}

func TestStartZenItem_BacklogOnlyDev_Moves(t *testing.T) {
	f := newFakePlane(t, stBacklog, "u-dev")
	if w := runHook(t, f, "zen-7", patEnv); w != "" {
		t.Fatalf("warning %q, want none", w)
	}
	body := f.onlyPatch(t)
	if body["state"] != stInProgress {
		t.Fatalf("state = %v, want %s", body["state"], stInProgress)
	}
	if _, ok := body["assignees"]; ok {
		t.Fatalf("body %v has assignees, want state only", body)
	}
}

// Slug with a suffix after the number still names ZEN-7.
func TestWtPlane_SlugSuffix(t *testing.T) {
	f := newFakePlane(t, stBacklog)
	if w := runHook(t, f, "zen-7-login-fix", patEnv); w != "" {
		t.Fatalf("warning %q", w)
	}
	if got := assigneesOf(t, f.onlyPatch(t)); !slices.Equal(got, []string{"u-dev"}) {
		t.Fatalf("assignees = %v", got)
	}
}

// Todo, the dev holds it → state only.
// Todo listing the dev and someone else: the dev may be only a watcher of another holder's item → warn, no PATCH.
func TestWtPlane_TodoDevAmongOthers_WarnsNoMove(t *testing.T) {
	f := newFakePlane(t, stTodo, "u-other", "u-dev")
	w := runHook(t, f, "zen-7", patEnv)
	if !strings.Contains(w, "holder unknown") {
		t.Fatalf("warning %q, want 'holder unknown'", w)
	}
	if len(f.patches) != 0 {
		t.Fatalf("patched %v", f.patches)
	}
}

func TestWtPlane_TodoHeldByDev_MovesState(t *testing.T) {
	f := newFakePlane(t, stTodo, "u-dev")
	if w := runHook(t, f, "zen-7", patEnv); w != "" {
		t.Fatalf("warning %q, want none", w)
	}
	body := f.onlyPatch(t)
	if body["state"] != stInProgress {
		t.Fatalf("state = %v", body["state"])
	}
	if _, ok := body["assignees"]; ok {
		t.Fatalf("body %v must not touch assignees", body)
	}
}

// SC-08: Todo held by someone else → warn, no PATCH.
func TestWtPlane_TodoHeldByOther_Warns(t *testing.T) {
	f := newFakePlane(t, stTodo, "u-other")
	w := runHook(t, f, "zen-7", patEnv)
	if w == "" || !strings.Contains(w, "ZEN-7") {
		t.Fatalf("warning %q, want one naming ZEN-7", w)
	}
	if len(f.patches) != 0 {
		t.Fatalf("patched %v, want none", f.patches)
	}
}

// In Progress and every later started state → nothing.
func TestWtPlane_StartedStates_NoOp(t *testing.T) {
	for _, st := range []string{stInProgress, stInReview, stOnStaging, stReady} {
		t.Run(st, func(t *testing.T) {
			f := newFakePlane(t, st, "u-other")
			if w := runHook(t, f, "zen-7", patEnv); w != "" {
				t.Fatalf("warning %q, want none", w)
			}
			if len(f.patches) != 0 {
				t.Fatalf("patched %v", f.patches)
			}
		})
	}
}

// Intake, Done, Cancelled → warn, no PATCH.
func TestWtPlane_ClosedOrIntake_Warns(t *testing.T) {
	for _, st := range []string{stTriage, stDone, stCancelled} {
		t.Run(st, func(t *testing.T) {
			f := newFakePlane(t, st)
			if w := runHook(t, f, "zen-7", patEnv); w == "" {
				t.Fatal("want a warning")
			}
			if len(f.patches) != 0 {
				t.Fatalf("patched %v", f.patches)
			}
		})
	}
}

func TestWtPlane_PlaneError_Warns(t *testing.T) {
	f := newFakePlane(t, stBacklog)
	f.fail = true
	w := runHook(t, f, "zen-7", patEnv)
	if w == "" {
		t.Fatal("want a warning")
	}
	if strings.Contains(w, "pat-secret") {
		t.Fatalf("warning leaks the PAT: %q", w)
	}
}

func TestWtPlane_NoPAT_Warns(t *testing.T) {
	f := newFakePlane(t, stBacklog)
	if w := runHook(t, f, "zen-7", func(string) string { return "" }); w == "" {
		t.Fatal("want a warning")
	}
	if len(f.requests) != 0 {
		t.Fatalf("requests %v, want none without a PAT", f.requests)
	}
}

// FR-09.3: a slug without a ZEN key never reaches Plane.
func TestWtPlane_NonZenSlug_NoRequest(t *testing.T) {
	f := newFakePlane(t, stBacklog)
	if w := runHook(t, f, "fix-login", patEnv); w != "" {
		t.Fatalf("warning %q", w)
	}
	if len(f.requests) != 0 {
		t.Fatalf("requests %v, want none", f.requests)
	}
}

// Todo with no assignee is unheld: moved, dev assigned.
func TestWtPlane_TodoUnheld_MovesAndAssigns(t *testing.T) {
	f := newFakePlane(t, stTodo)
	if w := runHook(t, f, "zen-7", patEnv); w != "" {
		t.Fatalf("warning %q, want none", w)
	}
	body := f.onlyPatch(t)
	if body["state"] != stInProgress {
		t.Fatalf("state = %v", body["state"])
	}
	if got := assigneesOf(t, body); !slices.Equal(got, []string{"u-dev"}) {
		t.Fatalf("assignees = %v, want [u-dev]", got)
	}
}

// Plane does not list the triage state: an unlisted state id warns as Intake-or-unlisted.
func TestWtPlane_StateNotInList_OwnWarning(t *testing.T) {
	f := newFakePlane(t, "st-unknown")
	w := runHook(t, f, "zen-7", patEnv)
	if !strings.Contains(w, "Intake or an unlisted state") {
		t.Fatalf("warning %q, want 'Intake or an unlisted state'", w)
	}
	if len(f.patches) != 0 {
		t.Fatalf("patched %v", f.patches)
	}
}

// An empty /users/me/ id warns and never PATCHes.
func TestWtPlane_EmptyUserID_WarnsNoPatch(t *testing.T) {
	f := newFakePlane(t, stBacklog)
	f.meID = ""
	w := runHook(t, f, "zen-7", patEnv)
	if w == "" || strings.Contains(w, "pat-secret") {
		t.Fatalf("warning %q", w)
	}
	if len(f.patches) != 0 {
		t.Fatalf("patched %v", f.patches)
	}
}

// A hung Plane is cut by the one overall deadline; the hook fails open.
func TestWtPlane_Deadline_FailsOpen(t *testing.T) {
	f := newFakePlane(t, stBacklog)
	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })
	f.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-hang:
		case <-r.Context().Done():
		}
	})
	saved := planeHookTimeoutVar
	planeHookTimeoutVar = 200 * time.Millisecond
	t.Cleanup(func() { planeHookTimeoutVar = saved })
	start := time.Now()
	w := runHook(t, f, "zen-7", patEnv)
	if w == "" || strings.Contains(w, "pat-secret") {
		t.Fatalf("warning %q", w)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("hook took %v", time.Since(start))
	}
}
