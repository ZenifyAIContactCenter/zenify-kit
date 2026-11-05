package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
)

// fakePlane answers GET /api/v1/users/me/ like Plane CE 1.4.2: 200 with the
// user for the good key, 403 for any other (measured 2026-11-06).
func fakePlane(t *testing.T, good string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/me/" || r.Header.Get("X-API-Key") != good {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"id":"u1","display_name":"namph","email":"namph@zenify.vn"}`))
	}))
	t.Cleanup(s.Close)
	return s
}

func answers(a ...string) func(string) (string, error) {
	return func(string) (string, error) {
		if len(a) == 0 {
			return "", nil
		}
		v := a[0]
		a = a[1:]
		return v, nil
	}
}

func savedPAT(t *testing.T, ws string) string {
	t.Helper()
	f, err := devenv.Load(devenv.Path(ws))
	if err != nil {
		t.Fatal(err)
	}
	v, _ := f.Get(planePATKey)
	return v
}

func TestAskPlanePAT_SavesAValidToken(t *testing.T) {
	ws, srv := t.TempDir(), fakePlane(t, "good")
	var opened string
	var out strings.Builder
	ok := askPlanePAT(&out, ws, srv.URL, planeTokenDeps{
		ask: answers(" good "), open: func(u string) error { opened = u; return nil }, check: checkPlanePAT,
	})
	if !ok || savedPAT(t, ws) != "good" {
		t.Fatalf("ok=%v saved=%q out=%s", ok, savedPAT(t, ws), out.String())
	}
	if opened != srv.URL+"/settings/profile/api-tokens/" || !strings.Contains(out.String(), opened) {
		t.Errorf("token page not opened/printed: %q\n%s", opened, out.String())
	}
	if !strings.Contains(out.String(), "namph") {
		t.Errorf("owner not shown: %s", out.String())
	}
}

func TestAskPlanePAT_RejectedThenValid(t *testing.T) {
	ws, srv := t.TempDir(), fakePlane(t, "good")
	var out strings.Builder
	if !askPlanePAT(&out, ws, srv.URL, planeTokenDeps{ask: answers("bad", "good"), open: func(string) error { return nil }, check: checkPlanePAT}) {
		t.Fatalf("second answer not taken:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "từ chối") { //znf:allow-lang
		t.Errorf("rejection not reported:\n%s", out.String())
	}
}

func TestAskPlanePAT_TwoRejectionsSaveNothing(t *testing.T) {
	ws, srv := t.TempDir(), fakePlane(t, "good")
	if askPlanePAT(&strings.Builder{}, ws, srv.URL, planeTokenDeps{ask: answers("bad", "worse", "good"), open: func(string) error { return nil }, check: checkPlanePAT}) {
		t.Fatal("a third try was taken")
	}
	if v := savedPAT(t, ws); v != "" {
		t.Errorf("saved %q", v)
	}
}

func TestAskPlanePAT_EmptySkips(t *testing.T) {
	ws, srv := t.TempDir(), fakePlane(t, "good")
	checked := false
	if askPlanePAT(&strings.Builder{}, ws, srv.URL, planeTokenDeps{ask: answers(""), open: func(string) error { return nil },
		check: func(string, string) (string, error) { checked = true; return "", nil }}) {
		t.Fatal("empty answer saved a token")
	}
	if checked {
		t.Error("empty answer sent to Plane")
	}
}

// Without a terminal up never asks: the patSet it hands registerPlaneMCP is
// the plain env/file lookup.
func TestPlanePATSetFor_NoTerminalNeverAsks(t *testing.T) {
	ws := t.TempDir()
	t.Setenv(planePATKey, "")
	asked := false
	d := planeTokenDeps{ask: func(string) (string, error) { asked = true; return "x", nil }, open: func(string) error { return nil },
		check: func(string, string) (string, error) { return "n", nil }}
	if planePATSetFor(&strings.Builder{}, ws, "https://plane.example", false, d)() || asked {
		t.Fatalf("asked=%v without a terminal", asked)
	}
	if !planePATSetFor(&strings.Builder{}, ws, "https://plane.example", true, d)() || !asked {
		t.Fatal("interactive run did not ask")
	}
}
