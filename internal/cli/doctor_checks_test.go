package cli

import (
	"strings"
	"testing"
)

func TestSecretPresenceNamesOnlyNeverLeaksValue(t *testing.T) {
	getenv := func(k string) string {
		if k == "MONGO_URL" {
			return "mongodb://secretuser:secretpass@10.0.0.1:27017"
		}
		return ""
	}
	c := secretPresenceCheck(getenv, "/nonexistent/settings.local.json")
	ok, detail := c.Run()
	if strings.Contains(detail, "secretpass") || strings.Contains(detail, "10.0.0.1") || strings.Contains(detail, "mongodb://") {
		t.Fatalf("FR-041 VIOLATION: secret value leaked into detail: %q", detail)
	}
	if !strings.Contains(detail, "MONGO_URL") {
		t.Fatalf("detail should name the key MONGO_URL: %q", detail)
	}
	_ = ok
}

func TestSecretPresenceReportsAbsent(t *testing.T) {
	c := secretPresenceCheck(func(string) string { return "" }, "/nonexistent/settings.local.json")
	ok, detail := c.Run()
	if ok {
		t.Fatal("no MONGO_URL anywhere → check should be not-ok")
	}
	if !strings.Contains(detail, "MONGO_URL") {
		t.Fatalf("should name the missing key: %q", detail)
	}
}

func TestToolPresenceReportsMissing(t *testing.T) {
	c := toolPresenceCheck([]string{"this-binary-does-not-exist-zzz"})
	ok, detail := c.Run()
	if ok {
		t.Fatal("a nonexistent tool should make the check not-ok")
	}
	if !strings.Contains(detail, "this-binary-does-not-exist-zzz") {
		t.Fatalf("should name the missing tool: %q", detail)
	}
}

func TestPlaywrightCheckIsReadOnlyRegistered(t *testing.T) {
	c := playwrightCheck()
	if c.Name != "playwright" {
		t.Fatalf("name: %q", c.Name)
	}
	// Run must not panic and must return a detail mentioning mcp state.
	_, detail := c.Run()
	if !strings.Contains(detail, "mcp=") {
		t.Fatalf("detail should report mcp state: %q", detail)
	}
}

func TestPluginCheckReportsMissing(t *testing.T) {
	c := pluginCheckAt(t.TempDir()) // dest rỗng → chưa sync
	ok, msg := c.Run()
	if ok {
		t.Fatal("plugin chưa sync mà báo ok")
	}
	if !strings.Contains(msg, "chưa cài") {
		t.Fatalf("msg không nêu trạng thái: %q", msg)
	}
}
