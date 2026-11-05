package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
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

// A redirect is not followed: X-API-Key would go to the other host.
func TestCheckPlanePAT_DoesNotFollowRedirect(t *testing.T) {
	leaked := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("X-API-Key") != ""
		_, _ = w.Write([]byte(`{"display_name":"x","email":"x@y"}`))
	}))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.RedirectHandler(other.URL+"/api/v1/users/me/", http.StatusFound))
	t.Cleanup(srv.Close)
	if _, err := checkPlanePAT(srv.URL, "tok"); err == nil || leaked {
		t.Fatalf("err=%v leaked=%v", err, leaked)
	}
}

// A 200 without the user's email is not a token Plane vouched for.
func TestCheckPlanePAT_RejectsAnswerWithoutEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	if owner, err := checkPlanePAT(srv.URL, "tok"); err == nil {
		t.Fatalf("accepted, owner=%q", owner)
	}
}

// Plane unreachable is not the token's fault: up stops asking instead of
// spending the second try, and says to run it again.
func TestAskPlanePAT_UnreachableStopsWithoutRetry(t *testing.T) {
	ws := t.TempDir()
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()
	asks := 0
	var out strings.Builder
	ok := askPlanePAT(&out, ws, base, planeTokenDeps{
		ask:  func(string) (string, error) { asks++; return "tok", nil },
		open: func(string) error { return nil }, check: checkPlanePAT,
	})
	if ok || asks != 1 || !strings.Contains(out.String(), "zenify up --apply") {
		t.Fatalf("ok=%v asks=%d out=%s", ok, asks, out.String())
	}
}

// Ctrl-C at the masked prompt restores the terminal before exiting, so echo
// does not stay off after zenify ends.
func TestRestoreOnSignal(t *testing.T) {
	sig := make(chan os.Signal, 1)
	restored, code := make(chan bool, 1), make(chan int, 1)
	stop := restoreOnSignal(sig, func() { restored <- true }, func(c int) { code <- c })
	sig <- os.Interrupt
	if !<-restored || <-code != 130 {
		t.Fatal("terminal not restored before exit")
	}
	stop()

	sig2 := make(chan os.Signal, 1)
	called := false
	stop = restoreOnSignal(sig2, func() { called = true }, func(int) { called = true })
	stop()
	if called {
		t.Error("restore ran without a signal")
	}
}
