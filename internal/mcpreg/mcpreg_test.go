package mcpreg

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/playwright"
)

type call struct {
	name string
	args []string
}

func fakeRun(getOut string, getErr error, calls *[]call) func(string, ...string) ([]byte, error) {
	return func(name string, args ...string) ([]byte, error) {
		*calls = append(*calls, call{name, args})
		if len(args) > 1 && args[1] == "get" {
			return []byte(getOut), getErr
		}
		return nil, nil
	}
}

const grafanaURL = "https://broker.example/mcp/grafana"

func TestBootstrap_AddsHTTPServer(t *testing.T) {
	var calls []call
	st, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun("No MCP server found with name: zenify-grafana", errors.New("exit 1"), &calls)})
	if err != nil || st != playwright.RegAbsent {
		t.Fatalf("st=%v err=%v", st, err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls=%v", calls)
	}
	a := calls[1]
	if a.name != "claude" || !reflect.DeepEqual([]string{a.args[0], a.args[1], a.args[2]}, []string{"mcp", "add-json", "zenify-grafana"}) || !reflect.DeepEqual(a.args[4:], []string{"-s", "user"}) {
		t.Fatalf("add argv=%v", a)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(a.args[3]), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"type": "http", "url": grafanaURL, "headersHelper": "zenify broker headers"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("json=%v", got)
	}
}

func TestBootstrap_MatchingNoop(t *testing.T) {
	var calls []call
	st, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun("zenify-grafana:\n  Scope: User config\n  Type: http\n  URL: "+grafanaURL+"\n", nil, &calls)})
	if err != nil || st != playwright.RegDesired {
		t.Fatalf("st=%v err=%v", st, err)
	}
	for _, c := range calls {
		if strings.Contains(strings.Join(c.args, " "), "add-json") {
			t.Fatalf("unexpected add: %v", c)
		}
	}
}

func TestBootstrap_KeepsCustom(t *testing.T) {
	for name, out := range map[string]string{
		"other helper":    "zenify-grafana:\n  Scope: User config (available in all your projects)\n  Type: http\n  URL: https://other.example/mcp\n  Headers helper: my-own-script\n",
		"no helper":       "zenify-grafana:\n  Scope: User config\n  Type: http\n  URL: https://other.example/mcp\n",
		"project scope":   "zenify-grafana:\n  Scope: Project config\n  Type: http\n  URL: https://old.example/mcp/grafana\n  Headers helper: zenify broker headers\n",
		"url is a prefix": "zenify-grafana:\n  Scope: User config\n  Type: http\n  URL: " + grafanaURL + "/x\n",
	} {
		var calls []call
		st, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun(out, nil, &calls)})
		if err != nil || st != playwright.RegCustom || len(calls) != 1 {
			t.Fatalf("%s: st=%v err=%v calls=%v", name, st, err, calls)
		}
	}
}

// A kit-written entry (user scope, the kit's headersHelper) pointing at an old
// broker URL is removed and re-added with the new URL.
func TestBootstrap_StaleKitEntryUpdated(t *testing.T) {
	var calls []call
	out := "zenify-grafana:\n  Scope: User config (available in all your projects)\n  Type: http\n  URL: https://old-broker.example/mcp/grafana\n  Headers helper: zenify broker headers\n"
	st, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun(out, nil, &calls)})
	if err != nil || st != playwright.RegLegacyDefault {
		t.Fatalf("st=%v err=%v", st, err)
	}
	if len(calls) != 3 || !reflect.DeepEqual(calls[1].args, []string{"mcp", "remove", "zenify-grafana", "-s", "user"}) || calls[2].args[1] != "add-json" || !strings.Contains(calls[2].args[3], grafanaURL) {
		t.Fatalf("calls=%v", calls)
	}
}

// A `claude mcp get` failure that is not "not found" adds nothing.
func TestBootstrap_GetFailureNoAdd(t *testing.T) {
	var calls []call
	_, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun("signal: killed", errors.New("timeout"), &calls)})
	if err == nil || len(calls) != 1 {
		t.Fatalf("err=%v calls=%v", err, calls)
	}
}

func TestBootstrap_AddFailureReturnsError(t *testing.T) {
	run := func(_ string, args ...string) ([]byte, error) {
		if args[1] == "get" {
			return []byte("No MCP server found with name: n"), errors.New("exit 1")
		}
		return nil, errors.New("boom")
	}
	if _, err := Bootstrap(Options{Spec: Spec{Name: "n", URL: grafanaURL}, Run: run}); err == nil || !strings.Contains(err.Error(), "register n MCP") {
		t.Fatalf("err=%v", err)
	}
}
