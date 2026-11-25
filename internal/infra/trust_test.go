package infra

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testPEM = "-----BEGIN CERTIFICATE-----\nMIIBfake\n-----END CERTIFICATE-----\n"

// Business rule: the cluster is addressed by the local kubeconfig and the CA
// comes out of the platform secret, decoded.
func TestFetchCA(t *testing.T) {
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		return []byte(base64.StdEncoding.EncodeToString([]byte(testPEM))), nil
	}}
	pem, err := FetchCA(context.Background(), r)
	if err != nil || string(pem) != testPEM {
		t.Fatalf("pem=%q err=%v", pem, err)
	}
	if !r.ran("kubectl --kubeconfig " + K3sKubeconfig + " -n zenify-system get secret zenify-local-ca") {
		t.Fatalf("calls: %v", r.calls)
	}
}

func TestFetchCARejectsEmptySecret(t *testing.T) {
	if _, err := FetchCA(context.Background(), &fakeRunner{}); err == nil {
		t.Fatal("want error when the secret has no certificate")
	}
}

// Business rule: trusting an empty CA would silently leave HTTPS unverified.
func TestInstallCARejectsEmptyPEM(t *testing.T) {
	r := &fakeRunner{}
	if _, err := InstallCA(context.Background(), r, t.TempDir(), nil); err == nil {
		t.Fatal("want error for an empty PEM")
	}
	if len(r.calls) != 0 {
		t.Fatalf("ran commands for an empty PEM: %v", r.calls)
	}
}

// nssFake fakes the host files and the NSS db: nss holds the PEM stored under the nickname.
func nssFake(files map[string]string, nss *string) func(call) ([]byte, error) {
	hf := writable(files)
	return func(c call) ([]byte, error) {
		switch {
		case c.name == "certutil" && c.args[0] == "-L":
			return []byte(*nss), nil
		case c.name == "certutil" && c.args[0] == "-A":
			*nss = c.stdin
			return nil, nil
		}
		return hf(c)
	}
}

type codedErr struct {
	msg  string
	code int
}

func (e codedErr) Error() string { return e.msg }
func (e codedErr) ExitCode() int { return e.code }

func newNSSFake(files map[string]string, nss *string) *fakeRunner {
	inner := nssFake(files, nss)
	return &fakeRunner{respond: func(c call) ([]byte, error) {
		if c.name == "certutil" && c.args[0] == "-L" && *nss == "" {
			return nil, codedErr{`certutil: Could not find cert: zenify-local-ca`, 255}
		}
		return inner(c)
	}}
}

// Business rule: a rerun with the same CA changes nothing; a new CA replaces the old one.
func TestInstallCAIdempotent(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".pki", "nssdb"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".pki", "nssdb", "cert9.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	files, nss := map[string]string{}, ""
	r := newNSSFake(files, &nss)
	changed, err := InstallCA(context.Background(), r, home, []byte(testPEM))
	if err != nil || !changed {
		t.Fatalf("first: changed=%v err=%v", changed, err)
	}
	if files[caSystemPath] != testPEM || nss != testPEM || !r.ran("sudo update-ca-certificates") {
		t.Fatalf("files=%v nss=%q calls=%v", files, nss, r.calls)
	}
	fi, err := os.Stat(filepath.Join(home, ".zenify", "infra", "ca.crt"))
	if err != nil || fi.Mode().Perm() != 0o644 {
		t.Fatalf("copy: %v %v", fi, err)
	}

	r2 := newNSSFake(files, &nss)
	changed, err = InstallCA(context.Background(), r2, home, []byte(testPEM))
	if err != nil || changed {
		t.Fatalf("second: changed=%v err=%v", changed, err)
	}
	if r2.ran("sudo update-ca-certificates") || r2.ran("certutil -A") {
		t.Fatalf("second run mutated: %v", r2.calls)
	}
}

// Business rule: a missing certutil must tell the user which package to install.
func TestInstallCAMissingCertutil(t *testing.T) {
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if c.name == "certutil" {
			return nil, &exec.Error{Name: "certutil", Err: exec.ErrNotFound}
		}
		return writable(map[string]string{})(c)
	}}
	_, err := InstallCA(context.Background(), r, t.TempDir(), []byte(testPEM))
	if err == nil || !strings.Contains(err.Error(), "libnss3-tools") {
		t.Fatalf("err = %v", err)
	}
}

// Business rule: the unit routes only the env zone and zenify.internal on cni0,
// never claims the default route, and no global resolved drop-in is written.
func TestConfigureSplitDNSUnit(t *testing.T) {
	files := map[string]string{}
	r := &fakeRunner{respond: writable(files)}
	changed, err := ConfigureSplitDNS(context.Background(), r, "quyentm.test")
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	u := files[dnsUnitPath]
	for _, want := range []string{
		"After=k3s.service", "Wants=k3s.service", "Type=oneshot", "RemainAfterExit=yes",
		"seq 1 120", "ip link show cni0",
		"ExecStart=/usr/bin/resolvectl dns cni0 10.43.200.53\n",
		"ExecStart=/usr/bin/resolvectl domain cni0 '~quyentm.test' '~zenify.internal'\n",
		"ExecStart=/usr/bin/resolvectl default-route cni0 false\n",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(u, want) {
			t.Fatalf("unit missing %q:\n%s", want, u)
		}
	}
	if strings.Contains(u, "default-route cni0 true") {
		t.Fatal("unit claims the default route")
	}
	if !r.ran("sudo systemctl enable --now "+dnsUnitName) || r.ran("sudo systemctl restart") {
		t.Fatalf("calls: %v", r.calls)
	}
	for _, c := range r.calls {
		if strings.Contains(c.line(), "resolved.conf.d") {
			t.Fatalf("touched a global drop-in: %s", c.line())
		}
	}
	for p := range files {
		if strings.Contains(p, "resolved.conf.d") {
			t.Fatalf("wrote %s", p)
		}
	}
}

// Business rule: a rerun with an unchanged unit does not restart it (no DNS blip);
// a changed domain does restart it.
func TestConfigureSplitDNSRerun(t *testing.T) {
	files := map[string]string{}
	if _, err := ConfigureSplitDNS(context.Background(), &fakeRunner{respond: writable(files)}, "quyentm.test"); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{respond: writable(files)}
	changed, err := ConfigureSplitDNS(context.Background(), r, "quyentm.test")
	if err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if r.ran("sudo systemctl restart") || r.ran("sudo systemctl enable") {
		t.Fatalf("calls: %v", r.calls)
	}
	r = &fakeRunner{respond: writable(files)}
	changed, err = ConfigureSplitDNS(context.Background(), r, "other.test")
	if err != nil || !changed || !r.ran("sudo systemctl restart "+dnsUnitName) {
		t.Fatalf("changed=%v err=%v calls=%v", changed, err, r.calls)
	}
}

// Business rule: the domain lands in a unit file, so a hostile value is refused.
func TestConfigureSplitDNSRejectsBadDomain(t *testing.T) {
	for _, d := range []string{"", "a b", "x'; rm -rf /; '", "Foo.test"} {
		if _, err := ConfigureSplitDNS(context.Background(), &fakeRunner{}, d); err == nil {
			t.Fatalf("accepted %q", d)
		}
	}
}

// Business rule: purge ignores an already-gone cni0 but surfaces any other resolvectl failure.
func TestRemoveTrustAndDNSErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	files := map[string]string{dnsUnitPath: "x", caSystemPath: testPEM}
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if c.line() == "sudo resolvectl revert cni0" {
			return nil, errors.New(`Failed to resolve interface "cni0": No such device`)
		}
		return writable(files)(c)
	}}
	if err := RemoveTrustAndDNS(context.Background(), r, "quyentm.test"); err != nil {
		t.Fatal(err)
	}
	if !r.ran("sudo systemctl disable --now "+dnsUnitName) || !r.ran("sudo rm -f "+caSystemPath) {
		t.Fatalf("calls: %v", r.calls)
	}
	r = &fakeRunner{respond: func(c call) ([]byte, error) {
		if c.line() == "sudo resolvectl revert cni0" {
			return nil, errors.New("Access denied")
		}
		return writable(map[string]string{})(c)
	}}
	if err := RemoveTrustAndDNS(context.Background(), r, "quyentm.test"); err == nil {
		t.Fatal("swallowed a revert failure")
	}
}

// writable extends hostFiles with the tee + mv that writeRootFile performs.
func writable(files map[string]string) func(call) ([]byte, error) {
	hf := hostFiles(files, "")
	return func(c call) ([]byte, error) {
		if c.name == "sudo" {
			switch c.args[0] {
			case "tee":
				files[c.args[1]] = c.stdin
				return nil, nil
			case "mv":
				files[c.args[3]] = files[c.args[2]]
				delete(files, c.args[2])
				return nil, nil
			}
		}
		return hf(c)
	}
}
