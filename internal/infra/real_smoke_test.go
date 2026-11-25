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

// TestRealBuild builds the core images on this host into the local registry,
// then checks every tag exists and a second pass skips them all. Needs a
// running platform; only runs with ZNF_INFRA_REAL_BUILD=1 and ZNF_INFRA_WS.
// Optional: ZNF_INFRA_DOMAIN, ZNF_INFRA_ONLY (comma-separated repos),
// ZNF_INFRA_REF_<repo with - as _> (ref override, e.g. ZNF_INFRA_REF_contact_center_web).
func TestRealBuild(t *testing.T) {
	if os.Getenv("ZNF_INFRA_REAL_BUILD") != "1" {
		t.Skip("set ZNF_INFRA_REAL_BUILD=1 and ZNF_INFRA_WS to build on the real host")
	}
	ws, domain := os.Getenv("ZNF_INFRA_WS"), os.Getenv("ZNF_INFRA_DOMAIN")
	if domain == "" {
		domain = "quyentm.test"
	}
	ts, err := CoreTargets(ws, domain, BaseSlug)
	if err != nil {
		t.Fatal(err)
	}
	only := map[string]bool{}
	for _, repo := range strings.Split(os.Getenv("ZNF_INFRA_ONLY"), ",") {
		if repo != "" {
			only[repo] = true
		}
	}
	refs := map[string]string{}
	for _, x := range ts {
		if v := os.Getenv("ZNF_INFRA_REF_" + strings.ReplaceAll(x.Repo, "-", "_")); v != "" {
			refs[x.Repo] = v
		}
	}
	ctx := context.Background()
	r := ExecBuildRunner()
	for pass := 1; pass <= 2; pass++ {
		for _, x := range ts {
			if len(only) > 0 && !only[x.Repo] {
				continue
			}
			res, err := BuildFromBase(ctx, r, ws, x, refs)
			t.Logf("pass %d %s:%s built=%v duration=%s err=%v", pass, res.Image, res.Tag, res.Built, res.Duration.Round(time.Second), err)
			if err != nil {
				t.Fatal(err)
			}
			if pass == 2 && res.Built {
				t.Fatalf("pass 2 rebuilt %s:%s: an existing tag must be skipped", res.Image, res.Tag)
			}
			if ok, err := tagExists(ctx, res.Image, res.Tag); err != nil || !ok {
				t.Fatalf("%s:%s not in the registry after build (exists=%v err=%v)", res.Image, res.Tag, ok, err)
			}
		}
	}
}
