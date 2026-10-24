package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/pwdocker"
)

func args(goos string) string {
	return strings.Join(BuildArgs(pwdocker.Options{GOOS: goos}, RunConfig{
		HarnessDir: "/tmp/h", E2EDir: "/repo/.znf/e2e", Port: 3339,
	}), " ")
}

func TestBuildArgs_MountsE2EDir(t *testing.T) {
	if !strings.Contains(args("linux"), "/repo/.znf/e2e:/harness/.znf/e2e") {
		t.Fatalf("must mount repo .znf/e2e, got %q", args("linux"))
	}
}

func TestBuildArgs_BaseURLPort(t *testing.T) {
	if !strings.Contains(args("darwin"), "BASE_URL=http://host.docker.internal:3339") {
		t.Fatal("BASE_URL sai port")
	}
}

func TestBuildArgs_StorageStateEnv(t *testing.T) {
	if !strings.Contains(args("linux"), "STORAGE_STATE=/tmp/znf-e2e-storage.json") {
		t.Fatal("must set STORAGE_STATE for e2e")
	}
}

func TestBuildArgs_LinuxAddHost(t *testing.T) {
	if !strings.Contains(args("linux"), "--add-host=host.docker.internal:host-gateway") {
		t.Fatal("linux missing --add-host")
	}
	if strings.Contains(args("darwin"), "--add-host") {
		t.Fatal("darwin has extra --add-host")
	}
}

func TestBuildArgs_BaseURL(t *testing.T) {
	got := strings.Join(BuildArgs(pwdocker.Options{GOOS: "darwin"}, RunConfig{
		HarnessDir: "/tmp/h", E2EDir: "/repo/.znf/e2e", BaseURL: "https://web-staging.zenify.vn",
	}), " ")
	if !strings.Contains(got, "BASE_URL=https://web-staging.zenify.vn") {
		t.Fatalf("BASE_URL missing: %q", got)
	}
	if strings.Contains(got, "BASE_URL=http://host.docker.internal") {
		t.Fatalf("must not use the host dev-server URL: %q", got)
	}
}

func TestBuildArgs_BaseURLNormalisedToOrigin(t *testing.T) {
	got := strings.Join(BuildArgs(pwdocker.Options{GOOS: "darwin"}, RunConfig{
		HarnessDir: "/tmp/h", E2EDir: "/repo/.znf/e2e", BaseURL: "https://web-staging.zenify.vn/some/path?x=1",
	}), " ")
	if !strings.Contains(got, "BASE_URL=https://web-staging.zenify.vn ") {
		t.Fatalf("BASE_URL not reduced to origin: %q", got)
	}
}

func TestCheckBaseURL(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) {
		if err := os.WriteFile(filepath.Join(dir, "e2e.config.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"apiBaseUrl":"x","baseUrls":["https://web-staging.zenify.vn"]}`)
	for _, u := range []string{"https://web-staging.zenify.vn", "https://web-staging.zenify.vn/login", "https://WEB-staging.zenify.vn/"} {
		if err := CheckBaseURL(dir, u); err != nil {
			t.Errorf("%s: want nil, got %v", u, err)
		}
	}
	for _, u := range []string{"https://zenify.vn", "http://web-staging.zenify.vn", "https://web-staging.zenify.vn:8443", "nope"} {
		err := CheckBaseURL(dir, u)
		if err == nil || !strings.Contains(err.Error(), "web-staging.zenify.vn") {
			t.Errorf("%s: want error listing allowed origin, got %v", u, err)
		}
	}
	write(`{"apiBaseUrl":"x"}`)
	if err := CheckBaseURL(dir, "https://web-staging.zenify.vn"); err == nil {
		t.Error("config without baseUrls must refuse")
	}
}
