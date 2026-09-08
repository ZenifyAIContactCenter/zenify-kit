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
	if !strings.Contains(strings.Join(args, " "), "--update-snapshots") {
		t.Errorf("Update=true must add --update-snapshots, got %v", args)
	}
}

func TestImage_UsesVersion(t *testing.T) {
	if !strings.Contains(Image(), PlaywrightVersion) {
		t.Errorf("Image() must embed PlaywrightVersion, got %q", Image())
	}
}
