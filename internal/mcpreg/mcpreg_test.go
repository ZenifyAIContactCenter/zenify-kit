package mcpreg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	st, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun(`No MCP server named "zenify-grafana".`, errors.New("exit 1"), &calls)})
	if err != nil || st != playwright.RegAbsent {
		t.Fatalf("st=%v err=%v", st, err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls=%v", calls)
	}
	a := calls[1]
	if a.name != "claude" || !reflect.DeepEqual([]string{a.args[0], a.args[1], a.args[2]}, []string{"mcp", "add-json", "zenify-grafana"}) || !reflect.DeepEqual(a.args[4:], []string{"-s", "local"}) {
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
	st, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun("zenify-grafana:\n  Scope: Local config (private to you in this project)\n  Type: http\n  URL: "+grafanaURL+"\n", nil, &calls)})
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
		"other path":      "zenify-grafana:\n  Scope: User config (available in all your projects)\n  Type: http\n  URL: https://other.example/mcp\n",
		"stdio":           "zenify-grafana:\n  Scope: User config\n  Status: ✓ Connected\n  Type: stdio\n  Command: my-grafana\n",
		"no helper":       "zenify-grafana:\n  Scope: User config\n  Type: http\n  URL: https://other.example/mcp\n",
		"project scope":   "zenify-grafana:\n  Scope: Project config (shared via .mcp.json)\n  Type: http\n  URL: https://old.example/mcp/grafana\n",
		"url is a prefix": "zenify-grafana:\n  Scope: User config\n  Type: http\n  URL: " + grafanaURL + "/x\n",
	} {
		var calls []call
		st, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun(out, nil, &calls)})
		if err != nil || st != playwright.RegCustom || len(calls) != 1 {
			t.Fatalf("%s: st=%v err=%v calls=%v", name, st, err, calls)
		}
	}
}

// A kit-written entry (user scope, http, same /mcp/<name> path) pointing at an
// old broker host is removed and re-added with the new URL. The fixture mirrors
// real `claude mcp get` output (2.1.300), which has no headersHelper line.
func TestBootstrap_StaleKitEntryUpdated(t *testing.T) {
	var calls []call
	out := "zenify-grafana:\n  Scope: Local config (private to you in this project)\n  Status: ✓ Connected\n  Type: http\n  URL: https://old-broker.example/mcp/grafana\n\nTo remove this server, run: claude mcp remove zenify-grafana -s user\n"
	st, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun(out, nil, &calls)})
	if err != nil || st != playwright.RegLegacyDefault {
		t.Fatalf("st=%v err=%v", st, err)
	}
	if len(calls) != 3 || !reflect.DeepEqual(calls[1].args, []string{"mcp", "remove", "zenify-grafana", "-s", "local"}) || calls[2].args[1] != "add-json" || !strings.Contains(calls[2].args[3], grafanaURL) {
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
			return []byte(`No MCP server named "n".`), errors.New("exit 1")
		}
		return nil, errors.New("boom")
	}
	if _, err := Bootstrap(Options{Spec: Spec{Name: "n", URL: grafanaURL}, Run: run}); err == nil || !strings.Contains(err.Error(), "register n MCP") {
		t.Fatalf("err=%v", err)
	}
}

// Real "absent" output of `claude mcp get` (2.1.300, exit 1) leads to an add.
func TestBootstrap_RealNotFoundWordingAdds(t *testing.T) {
	var calls []call
	out := `No MCP server named "zenify-grafana". Configured servers: grafana, playwright`
	st, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun(out, errors.New("exit status 1"), &calls)})
	if err != nil || st != playwright.RegAbsent || len(calls) != 2 || calls[1].args[1] != "add-json" {
		t.Fatalf("st=%v err=%v calls=%v", st, err, calls)
	}
}

var planeSpec = StdioSpec{Name: "zenify-plane", Command: "zenify", Args: []string{"plane-mcp"}}

func stdioBlock(extra, args string) string {
	return "zenify-plane:\n  Scope: Local config (private to you in this project)\n" + extra + "  Type: stdio\n  Command: zenify\n  Args: " + args + "\n  Environment:\n"
}

func TestBootstrapStdio_AddsWithoutEnv(t *testing.T) {
	var calls []call
	st, err := BootstrapStdio(planeSpec, fakeRun(`No MCP server named "zenify-plane".`, errors.New("exit 1"), &calls))
	if err != nil || st != playwright.RegAbsent {
		t.Fatalf("st=%v err=%v", st, err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls=%v", calls)
	}
	a := calls[1]
	if a.name != "claude" || !reflect.DeepEqual(a.args[:3], []string{"mcp", "add-json", "zenify-plane"}) || !reflect.DeepEqual(a.args[4:], []string{"-s", "local"}) {
		t.Fatalf("add argv=%v", a)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(a.args[3]), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"type": "stdio", "command": "zenify", "args": []any{"plane-mcp"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("json=%v", got)
	}
}

func noMutation(t *testing.T, calls []call) {
	t.Helper()
	for _, c := range calls {
		j := strings.Join(c.args, " ")
		if strings.Contains(j, "add-json") || strings.Contains(j, "remove") {
			t.Fatalf("unexpected mutation: %v", c)
		}
	}
}

func TestBootstrapStdio_DesiredNoop(t *testing.T) {
	var calls []call
	st, err := BootstrapStdio(planeSpec, fakeRun(stdioBlock("", "plane-mcp"), nil, &calls))
	if err != nil || st != playwright.RegDesired {
		t.Fatalf("st=%v err=%v", st, err)
	}
	noMutation(t, calls)
}

// A failed health check prints Status/Issue lines but exits 0; ownership is
// decided by Type/Command/Args alone.
func TestBootstrapStdio_DesiredWithFailedStatus(t *testing.T) {
	var calls []call
	out := stdioBlock("  Status: ✘ Failed to connect\n  Issue: spawn zenify ENOENT\n", "plane-mcp")
	st, err := BootstrapStdio(planeSpec, fakeRun(out, nil, &calls))
	if err != nil || st != playwright.RegDesired {
		t.Fatalf("st=%v err=%v", st, err)
	}
	noMutation(t, calls)
}

func TestBootstrapStdio_StaleArgsReplaced(t *testing.T) {
	var calls []call
	st, err := BootstrapStdio(planeSpec, fakeRun(stdioBlock("", "plane-mcp --old"), nil, &calls))
	if err != nil || st != playwright.RegLegacyDefault {
		t.Fatalf("st=%v err=%v", st, err)
	}
	if len(calls) != 3 || calls[1].args[1] != "remove" || calls[2].args[1] != "add-json" {
		t.Fatalf("calls=%v", calls)
	}
}

func TestBootstrapStdio_CustomKept(t *testing.T) {
	for name, out := range map[string]string{
		"http":        "zenify-plane:\n  Scope: User config\n  Type: http\n  URL: https://x/mcp\n",
		"other cmd":   "zenify-plane:\n  Scope: User config\n  Type: stdio\n  Command: uvx\n  Args: plane-mcp\n",
		"project cfg": "zenify-plane:\n  Scope: Project config\n  Type: stdio\n  Command: zenify\n  Args: other\n",
	} {
		var calls []call
		st, err := BootstrapStdio(planeSpec, fakeRun(out, nil, &calls))
		if err != nil || st != playwright.RegCustom {
			t.Fatalf("%s: st=%v err=%v", name, st, err)
		}
		noMutation(t, calls)
	}
}

func TestBootstrapStdio_GetFailureIsError(t *testing.T) {
	var calls []call
	if _, err := BootstrapStdio(planeSpec, fakeRun("boom", errors.New("timeout"), &calls)); err == nil || len(calls) != 1 {
		t.Fatalf("err=%v calls=%v", err, calls)
	}
}

// A get killed mid health check still printed the registration: when it
// already matches, nothing is touched and no error surfaces.
func TestBootstrapStdio_DesiredDespiteGetError(t *testing.T) {
	var calls []call
	st, err := BootstrapStdio(planeSpec, fakeRun(stdioBlock("", "plane-mcp"), errors.New("signal: killed"), &calls))
	if err != nil || st != playwright.RegDesired {
		t.Fatalf("st=%v err=%v", st, err)
	}
	noMutation(t, calls)
}

// A get that timed out with no output is an error, never an add/remove.
func TestBootstrapStdio_TimeoutNoMutation(t *testing.T) {
	var calls []call
	_, err := BootstrapStdio(planeSpec, fakeRun("", fmt.Errorf("%w: signal: killed", context.DeadlineExceeded), &calls))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	noMutation(t, calls)
}

const userScope = "Scope: User config (available in all your projects)"
const localScope = "Scope: Local config (private to you in this project)"

func TestBootstrap_UserScopedSameURLAddsLocal(t *testing.T) {
	var calls []call
	out := "zenify-grafana:\n  " + userScope + "\n  Type: http\n  URL: " + grafanaURL + "\n"
	st, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun(out, nil, &calls)})
	if err != nil || st != playwright.RegLegacyDefault {
		t.Fatalf("st=%v err=%v", st, err)
	}
	if len(calls) != 2 || calls[1].args[1] != "add-json" || !reflect.DeepEqual(calls[1].args[4:], []string{"-s", "local"}) {
		t.Fatalf("calls=%v", calls)
	}
	for _, c := range calls {
		if c.args[1] == "remove" {
			t.Fatalf("user entry must not be removed here: %v", c)
		}
	}
}

func TestBootstrap_UserScopedStaleHostAddsLocal(t *testing.T) {
	var calls []call
	out := "zenify-grafana:\n  " + userScope + "\n  Type: http\n  URL: https://old.example/mcp/grafana\n"
	st, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun(out, nil, &calls)})
	if err != nil || st != playwright.RegLegacyDefault || len(calls) != 2 || calls[1].args[1] != "add-json" {
		t.Fatalf("st=%v err=%v calls=%v", st, err, calls)
	}
}

func TestBootstrap_LocalDesired(t *testing.T) {
	var calls []call
	out := "zenify-grafana:\n  " + localScope + "\n  Type: http\n  URL: " + grafanaURL + "\n"
	st, err := Bootstrap(Options{Spec: Spec{Name: "zenify-grafana", URL: grafanaURL}, Run: fakeRun(out, nil, &calls)})
	if err != nil || st != playwright.RegDesired || len(calls) != 1 {
		t.Fatalf("st=%v err=%v calls=%v", st, err, calls)
	}
}

func TestBootstrapStdio_UserScopedAddsLocal(t *testing.T) {
	var calls []call
	out := "zenify-plane:\n  " + userScope + "\n  Type: stdio\n  Command: zenify\n  Args: plane-mcp\n"
	st, err := BootstrapStdio(planeSpec, fakeRun(out, nil, &calls))
	if err != nil || st != playwright.RegLegacyDefault || len(calls) != 2 || calls[1].args[1] != "add-json" || !reflect.DeepEqual(calls[1].args[4:], []string{"-s", "local"}) {
		t.Fatalf("st=%v err=%v calls=%v", st, err, calls)
	}
}

func TestRemoveUserScoped(t *testing.T) {
	var got []string
	ok, err := RemoveUserScoped("zenify-grafana", func(n string, a ...string) ([]byte, error) {
		got = append([]string{n}, a...)
		return nil, nil
	})
	if err != nil || !ok || !reflect.DeepEqual(got, []string{"claude", "mcp", "remove", "zenify-grafana", "-s", "user"}) {
		t.Fatalf("ok=%v err=%v got=%v", ok, err, got)
	}
}

func TestRemoveUserScoped_NotFound(t *testing.T) {
	ok, err := RemoveUserScoped("zenify-grafana", func(string, ...string) ([]byte, error) {
		return []byte(`No user-scoped MCP server found with name: zenify-grafana`), errors.New("exit status 1")
	})
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	ok, err = RemoveUserScoped("x", func(string, ...string) ([]byte, error) { return []byte("boom"), errors.New("exit status 2") })
	if err == nil || ok {
		t.Fatalf("real failure must error: ok=%v err=%v", ok, err)
	}
}
