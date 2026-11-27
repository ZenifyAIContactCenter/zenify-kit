package infra

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestEnsureBuildctlRejectsChecksumMismatch(t *testing.T) {
	home := t.TempDir()
	asset := fmt.Sprintf("buildkit-%s.linux-%s.tar.gz", BuildkitVersion, runtime.GOARCH)
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		switch c.name {
		case "gh":
			return []byte(`{"assets":[{"name":"` + asset + `","digest":"sha256:00","browser_download_url":"https://example.invalid/x"}]}`), nil
		case "curl":
			return nil, os.WriteFile(c.args[2], []byte("tampered"), 0o600)
		}
		return nil, errors.New("unexpected " + c.line())
	}}
	_, err := EnsureBuildctl(context.Background(), r, home)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want a checksum mismatch: a tampered download would run as buildctl", err)
	}
	if _, serr := os.Stat(buildctlPath(home)); !errors.Is(serr, os.ErrNotExist) {
		t.Fatal("unverified buildctl installed")
	}
}
