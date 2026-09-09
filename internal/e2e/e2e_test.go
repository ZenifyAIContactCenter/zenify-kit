package e2e

import (
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
