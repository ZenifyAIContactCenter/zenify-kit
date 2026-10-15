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
	var calls []call
	st, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun("zenify-grafana:\n  Type: http\n  URL: https://other.example/mcp\n", nil, &calls)})
	if err != nil || st != playwright.RegCustom {
		t.Fatalf("st=%v err=%v", st, err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls=%v", calls)
	}
}

func TestBootstrap_AddFailureReturnsError(t *testing.T) {
	run := func(_ string, args ...string) ([]byte, error) {
		return nil, errors.New("boom")
	}
	if _, err := Bootstrap(Options{Spec: Spec{Name: "n", URL: grafanaURL}, Run: run}); err == nil {
		t.Fatal("want error")
	}
}
