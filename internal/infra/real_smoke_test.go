package infra

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestRealInstall drives the whole install on this host. It mutates the host
// (root files, k3s restart, cluster objects), so it only runs with
// ZNF_INFRA_REAL=1 and never in CI. Optional: ZNF_INFRA_DOMAIN, ZNF_INFRA_CHART_REF.
func TestRealInstall(t *testing.T) {
	if os.Getenv("ZNF_INFRA_REAL") != "1" {
		t.Skip("set ZNF_INFRA_REAL=1 to run against the real host")
	}
	domain, ref := os.Getenv("ZNF_INFRA_DOMAIN"), os.Getenv("ZNF_INFRA_CHART_REF")
	if domain == "" {
		domain = "quyentm.test"
	}
	if ref == "" {
		ref = "main"
	}
	ctx := context.Background()
	r := ExecRunner()

	changed, err := InstallK3s(ctx, r)
	t.Logf("InstallK3s changed=%v err=%v", changed, err)
	if err != nil {
		t.Fatal(err)
	}
	changed, err = InstallArgo(ctx, r)
	t.Logf("InstallArgo changed=%v err=%v", changed, err)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		t.Fatalf("gh auth token: %v", err)
	}
	if err := SetRepoCredential(ctx, r, strings.TrimSpace(string(tok))); err != nil {
		t.Fatal(err)
	}
	t.Log("SetRepoCredential ok")
	if err := ApplyPlatform(ctx, r, domain, ref); err != nil {
		t.Fatal(err)
	}
	t.Log("ApplyPlatform ok")
	if err := WaitApp(ctx, r, PlatformApp, 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	t.Log("WaitApp: Synced + Healthy")
}
