package infra

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Business rule (public binary): workspace truth lives only in infra.json; a
// missing file must stop with the file and the fields it needs, never fall
// back to compiled-in repos.
func TestLoadWorkspaceMissingOrIncomplete(t *testing.T) {
	p := filepath.Join(t.TempDir(), WorkspaceFile)
	_, err := LoadWorkspace(p)
	if err == nil || !strings.Contains(err.Error(), p) || !strings.Contains(err.Error(), "helmRepoURL, builds, envApps and seed") {
		t.Fatalf("missing file err = %v", err)
	}
	if err := os.WriteFile(p, []byte(`{"helmRepoURL":"https://git.example.test/c.git","envApps":[{"key":"env-x","phase":3}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = LoadWorkspace(p)
	for _, want := range []string{p, "builds", "seed.repo", "seed.accounts", "envApps[0].key", "envApps[0].phase", "envApps[].seed"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("incomplete file err = %v, want it to name %s", err, want)
		}
	}
	if err := os.WriteFile(p, []byte(`{"helmRepoURL":"x","typo":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWorkspace(p); err == nil || !strings.Contains(err.Error(), "typo") {
		t.Fatalf("unknown field err = %v", err)
	}
}

// Business rule: a secret already exported must not be blocked by an
// unreadable env store: the build only reads the store for a missing key.
func TestSecretEnvEnvWinsOverUnreadableStore(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".zenify", ".env"), 0o700); err != nil { // a dir: Load fails
		t.Fatal(err)
	}
	t.Setenv("NPM_TOKEN", "from-env")
	used, env, _, err := secretEnv(context.Background(), &fakeRunner{}, ws, []string{"npm_token"}, nil)
	if err != nil || len(used) != 1 || env[0] != "NPM_TOKEN=from-env" {
		t.Fatalf("used=%v env=%v err=%v", used, env, err)
	}
	t.Setenv("NPM_TOKEN", "")
	if _, _, _, err := secretEnv(context.Background(), &fakeRunner{}, ws, []string{"npm_token"}, nil); err == nil {
		t.Fatal("a key the env lacks must surface the unreadable store")
	}
}

// Business rule: the log masks secrets line by line, so a secret spanning lines
// (a private key) would leak its lines: it is refused, named, never printed.
func TestSecretEnvRefusesMultiLineSecret(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("SECRET_X", "alpha\nbeta")
	_, _, _, err := secretEnv(context.Background(), &fakeRunner{}, ws, []string{"secret_x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "SECRET_X spans several lines") || strings.Contains(err.Error(), "alpha") {
		t.Fatalf("err = %v, want the key named without its value", err)
	}
}

// Business rule: a cut through a secret in a long build log must not leak a
// fragment of it: redaction runs before the stderr tail is cut.
func TestRunEnvRedactsBeforeTruncating(t *testing.T) {
	token := "tok_" + strings.Repeat("Z", 36) // 40 bytes
	script := `printf '%s' "$SECRET_X" >&2; head -c 3961 /dev/zero | tr '\0' x >&2; exit 1`
	_, err := ExecBuildRunner().RunEnv(context.Background(), []string{"SECRET_X=" + token}, "sh", "-c", script)
	if err == nil {
		t.Fatal("want failure")
	}
	if strings.Contains(err.Error(), token[1:20]) || strings.Contains(err.Error(), strings.Repeat("Z", 20)) {
		t.Fatalf("token fragment leaked: ...%s", err.Error()[len(err.Error())-80:])
	}
}

// Business rule: firewall files written by a run that failed before the
// reload must be reloaded by the next run, even though the files now match.
func TestInstallK3sReloadsFirewallAfterFailedReload(t *testing.T) {
	files := desiredHostFiles(t)
	delete(files, nftPath)
	base := writable(files)
	fail := true
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if c.line() == "sudo systemctl daemon-reload" && fail {
			return nil, errors.New("daemon-reload failed")
		}
		return base(c)
	}}
	if _, err := InstallK3s(context.Background(), r); err == nil {
		t.Fatal("want the daemon-reload error")
	}
	if _, ok := files[fwMarker]; !ok {
		t.Fatal("no firewall marker left after a failed reload")
	}
	fail = false
	r2 := &fakeRunner{respond: base}
	if _, err := InstallK3s(context.Background(), r2); err != nil {
		t.Fatal(err)
	}
	if !r2.ran("sudo systemctl restart "+firewallUnit) || !r2.ran("sudo rm -f "+fwMarker) {
		t.Fatalf("rerun did not reload the firewall: %v", r2.calls)
	}
}

// Business rule: down --purge must not remove the firewall (and report
// success) while k3s is still installed without its uninstall script.
func TestUninstallK3sMissingScriptButInstalled(t *testing.T) {
	r := &fakeRunner{respond: hostFiles(map[string]string{}, K3sVersion)}
	err := UninstallK3s(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), "k3s-uninstall.sh is missing") {
		t.Fatalf("err = %v", err)
	}
	if r.ran("sudo systemctl disable") || r.ran("sudo rm -f "+unitPath) {
		t.Fatalf("firewall removed while k3s can still start: %v", r.calls)
	}
	gone := &fakeRunner{respond: hostFiles(map[string]string{}, "")}
	if err := UninstallK3s(context.Background(), gone); err != nil {
		t.Fatalf("k3s already gone: %v", err)
	}
}

// Business rule: a CA file written by a run that failed before
// update-ca-certificates must still get into the bundle on the next run, and
// a purge that failed after deleting the file must still drop it.
func TestInstallCARegeneratesBundleWhenMissing(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".pki", "nssdb"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".pki", "nssdb", "cert9.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	files, nss := map[string]string{caSystemPath: testPEM}, testPEM // file in place, bundle empty
	if err := os.WriteFile(caBundlePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r := newNSSFake(files, &nss)
	if _, err := InstallCA(context.Background(), r, home, []byte(testPEM)); err != nil {
		t.Fatal(err)
	}
	if !r.ran("sudo update-ca-certificates") {
		t.Fatalf("bundle not regenerated: %v", r.calls)
	}

	delete(files, caSystemPath) // purge deleted the file, the bundle still has it
	r2 := &fakeRunner{respond: writable(files)}
	if err := RemoveTrustAndDNS(context.Background(), r2, home); err != nil {
		t.Fatal(err)
	}
	if !r2.ran("sudo update-ca-certificates") {
		t.Fatalf("stale bundle entry not dropped: %v", r2.calls)
	}
}

// Business rule: status must not report the CA trusted when a store holds a
// different CA (an interrupted install after the CA was regenerated).
func TestCATrustedComparesEveryStore(t *testing.T) {
	home := t.TempDir()
	write := func(p, s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(infraDir(home), caCopyName), testPEM)
	write(caSystemReadPath, testPEM)
	write(caBundlePath, testPEM)
	write(filepath.Join(nssDir(home), "cert9.db"), "")
	nss := func(pemOut string) *fakeRunner {
		return &fakeRunner{respond: func(c call) ([]byte, error) {
			if strings.Contains(c.line(), "-n "+caNSSNick+" -a") {
				return []byte(pemOut), nil
			}
			return []byte("Certificate Nickname   Trust Attributes\n" + caNSSNick + "                       C,,\n"), nil
		}}
	}
	if ok, err := CATrusted(context.Background(), nss(testPEM), home); err != nil || !ok {
		t.Fatalf("all stores match: ok=%v err=%v", ok, err)
	}
	if ok, err := CATrusted(context.Background(), nss(genCert(t, true)), home); err != nil || ok {
		t.Fatalf("NSS holds another CA: ok=%v err=%v", ok, err)
	}
	write(caBundlePath, "")
	if ok, err := CATrusted(context.Background(), nss(testPEM), home); err != nil || ok {
		t.Fatalf("bundle lacks the CA: ok=%v err=%v", ok, err)
	}
}

// Business rule: two infra runs at once would race on temp worktrees and root
// files; the second must stop naming the lock.
func TestLockHostExclusive(t *testing.T) {
	home := t.TempDir()
	unlock, err := LockHost(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LockHost(home); err == nil || !strings.Contains(err.Error(), "infra.lock") {
		t.Fatalf("second lock err = %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	again, err := LockHost(home)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	if err := again(); err != nil {
		t.Fatal(err)
	}
}

// Business rule: DNS labels are at most 63 bytes, and the domain must leave
// room for an app host label; a longer one would give envs invalid names.
func TestValidateDomainLabelLength(t *testing.T) {
	if err := ValidateDomain(strings.Repeat("a", 63) + ".test"); err != nil {
		t.Fatalf("63-byte label: %v", err)
	}
	if err := ValidateDomain(strings.Repeat("a", 64) + ".test"); err == nil {
		t.Fatal("64-byte label accepted")
	}
	long := strings.Repeat(strings.Repeat("a", 60)+".", 3) + "test" // 187 bytes: fits
	if err := ValidateDomain(long); err != nil {
		t.Fatalf("187-byte domain: %v", err)
	}
	if err := ValidateDomain(strings.Repeat("a", 60) + "." + long); err == nil {
		t.Fatal("a domain with no room for an app host accepted")
	}
}
