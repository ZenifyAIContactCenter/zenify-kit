package visual

import (
	"strings"
	"testing"
)

func TestBuildArgs_DarwinNoAddHost(t *testing.T) {
	o := Options{GOOS: "darwin", Getenv: func(string) string { return "" }}
	args := BuildArgs(o, RunConfig{HarnessDir: "/tmp/h", SnapshotsDir: "/repo/.znf/visual", Port: 3201})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "run") || !strings.Contains(joined, "--rm") {
		t.Fatalf("expected docker run --rm, got %q", joined)
	}
	if !strings.Contains(joined, Image()) {
		t.Errorf("expected pinned image %q in %q", Image(), joined)
	}
	if !strings.Contains(joined, "BASE_URL=http://host.docker.internal:3201") {
		t.Errorf("expected BASE_URL with port, got %q", joined)
	}
	if strings.Contains(joined, "--add-host") {
		t.Errorf("darwin must NOT set --add-host, got %q", joined)
	}
}

func TestBuildArgs_LinuxAddHost(t *testing.T) {
	o := Options{GOOS: "linux", Getenv: func(string) string { return "" }}
	args := BuildArgs(o, RunConfig{HarnessDir: "/tmp/h", SnapshotsDir: "/repo/.znf/visual", Port: 3201})
	if !strings.Contains(strings.Join(args, " "), "--add-host=host.docker.internal:host-gateway") {
		t.Errorf("linux must set --add-host host-gateway, got %v", args)
	}
}

func TestBuildArgs_UpdateFlag(t *testing.T) {
	o := Options{GOOS: "darwin", Getenv: func(string) string { return "" }}
	args := BuildArgs(o, RunConfig{HarnessDir: "/tmp/h", SnapshotsDir: "/repo/.znf/visual", Port: 3201, Update: true})
	cmd := args[len(args)-1]
	// The default update mode skips an aria snapshot that has no baseline yet,
	// so a missing-only pass must come before the rewrite pass.
	missing := strings.Index(cmd, "--update-snapshots=missing")
	changed := strings.Index(cmd, "--update-snapshots=changed")
	if missing < 0 || changed < missing {
		t.Errorf("Update=true must run a missing pass, then a changed pass, got %q", cmd)
	}
}

func TestImage_UsesVersion(t *testing.T) {
	if !strings.Contains(Image(), PlaywrightVersion) {
		t.Errorf("Image() must embed PlaywrightVersion, got %q", Image())
	}
}
