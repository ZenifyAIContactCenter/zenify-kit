package infra

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	caSecretName  = "zenify-local-ca" //nolint:gosec // G101 -- a Secret object name, not a credential
	caSecretKey   = "ca.crt"
	caSystemPath  = "/usr/local/share/ca-certificates/zenify-local-ca.crt"
	caNSSNick     = "zenify-local-ca"
	dnsUnitName   = "zenify-infra-dns.service"
	dnsUnitPath   = "/etc/systemd/system/" + dnsUnitName
	caCopyName    = "ca.crt"
	certutilHint  = "certutil not found: install package libnss3-tools"
	domainPattern = `^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`
)

var domainRe = regexp.MustCompile(domainPattern)

// maxDomainLen leaves room for one more 63-byte label (an app host such as
// <svc>-<slug>.) within the 253-byte DNS name limit.
const maxDomainLen = 253 - 64

// validDomainName requires lower-case DNS labels of 1-63 bytes and a name
// short enough that every app host under it is still a valid DNS name.
func validDomainName(d string) error {
	if len(d) > maxDomainLen || !domainRe.MatchString(d) {
		return fmt.Errorf("invalid domain %q: lower-case DNS labels (a-z, 0-9, -), at most %d bytes", d, maxDomainLen)
	}
	for _, l := range strings.Split(d, ".") {
		if len(l) > 63 {
			return fmt.Errorf("invalid domain %q: label %q is longer than 63 bytes", d, l)
		}
	}
	return nil
}

// FetchCA reads the local CA certificate (PEM) from the cluster.
func FetchCA(ctx context.Context, r Runner) ([]byte, error) {
	out, err := kubectl(ctx, r, nil, "-n", PlatformNamespace, "get", "secret", caSecretName,
		"-o", `jsonpath={.data.ca\.crt}`)
	if err != nil {
		return nil, fmt.Errorf("fetch CA: read secret %s/%s: %w", PlatformNamespace, caSecretName, err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		return nil, fmt.Errorf("fetch CA: decode %s: %w", caSecretKey, err)
	}
	clean, err := validateCA(raw)
	if err != nil {
		return nil, fmt.Errorf("fetch CA: secret %s/%s key %s: %w", PlatformNamespace, caSecretName, caSecretKey, err)
	}
	return clean, nil
}

// validateCA requires exactly one PEM block, a parseable CA certificate, and
// nothing else, and returns it re-encoded: a trusted root must never be a
// byte-for-byte copy of unvalidated input.
func validateCA(raw []byte) ([]byte, error) {
	block, rest := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("more than one PEM block or trailing data: want exactly one CA certificate")
	}
	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("PEM block is %q, want CERTIFICATE", block.Type)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}
	if !cert.IsCA {
		return nil, errors.New("certificate is not a CA")
	}
	return pem.EncodeToMemory(block), nil
}

func samePEM(a, b []byte) bool {
	return bytes.Equal(bytes.Join(bytes.Fields(a), nil), bytes.Join(bytes.Fields(b), nil))
}

// InstallCA makes the host and the user's NSS database (Chrome/Chromium; Firefox profiles keep their own store) trust
// the CA, and keeps a copy at ~/.zenify/infra/ca.crt. changed is false when
// everything already held this exact CA.
func InstallCA(ctx context.Context, r Runner, home string, rawPEM []byte) (changed bool, err error) {
	crt, err := validateCA(rawPEM)
	if err != nil {
		return false, fmt.Errorf("install CA: %w", err)
	}

	cur, exists, err := readRootFile(ctx, r, caSystemPath)
	if err != nil {
		return false, fmt.Errorf("install CA: %w", err)
	}
	fileOK := exists && samePEM(cur, crt)
	if !fileOK {
		if err := writeRootFile(ctx, r, caSystemPath, crt); err != nil {
			return false, fmt.Errorf("install CA: %w", err)
		}
		changed = true
	}
	// an earlier run may have written the file and failed before the bundle
	// was regenerated: check the bundle, not only the file
	inBundle, err := bundleHas(crt)
	if err != nil {
		return changed, fmt.Errorf("install CA: %w", err)
	}
	if !fileOK || !inBundle {
		if _, err := r.Run(ctx, nil, "sudo", "update-ca-certificates"); err != nil {
			return true, fmt.Errorf("install CA: update-ca-certificates: %w", err)
		}
		changed = true
	}

	nssChanged, err := installNSS(ctx, r, home, crt)
	if err != nil {
		return changed, err
	}

	copied, err := saveCACopy(home, crt)
	if err != nil {
		return changed || nssChanged, err
	}
	return changed || nssChanged || copied, nil
}

// saveCACopy keeps a world-readable copy at ~/.zenify/infra/ca.crt.
func saveCACopy(home string, crt []byte) (bool, error) {
	copyPath := filepath.Join(infraDir(home), caCopyName)
	have, err := os.ReadFile(copyPath) //nolint:gosec // G304 -- path under the user home, built from fixed names
	if err == nil && samePEM(have, crt) {
		return false, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("install CA: read %s: %w", copyPath, err)
	}
	if err := writeFileAtomic(home, caCopyName, crt); err != nil {
		return false, fmt.Errorf("install CA: %w", err)
	}
	if err := os.Chmod(copyPath, 0o644); err != nil { //nolint:gosec // G302 -- a public CA certificate, meant to be world-readable
		return true, fmt.Errorf("install CA: chmod %s: %w", copyPath, err)
	}
	return true, nil
}

func nssDir(home string) string { return filepath.Join(home, ".pki", "nssdb") }

// installNSS adds the CA to the user's NSS DB, creating the DB when absent.
func installNSS(ctx context.Context, r Runner, home string, crt []byte) (bool, error) {
	db := "sql:" + nssDir(home)
	if _, err := os.Stat(filepath.Join(nssDir(home), "cert9.db")); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("install CA: stat NSS db: %w", err)
		}
		if err := os.MkdirAll(nssDir(home), 0o700); err != nil {
			return false, fmt.Errorf("install CA: mkdir %s: %w", nssDir(home), err)
		}
		if _, err := r.Run(ctx, nil, "certutil", "-N", "-d", db, "--empty-password"); err != nil {
			return false, certutilErr("create NSS db", err)
		}
	}
	have, err := r.Run(ctx, nil, "certutil", "-L", "-d", db, "-n", caNSSNick, "-a")
	switch {
	case err == nil && samePEM(have, crt):
		return false, nil
	case err == nil:
		// a different CA under our nickname (the CA was regenerated): replace it
		if _, err := r.Run(ctx, nil, "certutil", "-D", "-d", db, "-n", caNSSNick); err != nil {
			return false, certutilErr("delete stale NSS cert", err)
		}
	case !nssNotFound(err):
		return false, certutilErr("list NSS certs", err)
	}
	if _, err := r.Run(ctx, bytes.NewReader(crt), "certutil", "-A", "-d", db, "-t", "C,,", "-n", caNSSNick); err != nil {
		return false, certutilErr("add NSS cert", err)
	}
	return true, nil
}

// nssNotFound: certutil exits 255 with "could not find" when the nickname is absent.
func nssNotFound(err error) bool {
	return exitCode(err) > 0 && strings.Contains(strings.ToLower(err.Error()), "could not find")
}

func certutilErr(step string, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("install CA: %s: %s", step, certutilHint)
	}
	return fmt.Errorf("install CA: %s: %w", step, err)
}

func dnsUnit(domain string) ([]byte, error) {
	if err := validDomainName(domain); err != nil {
		return nil, err
	}
	return renderAsset("dns.service.tmpl", map[string]string{"DNSClusterIP": DNSClusterIP, "Domain": domain})
}

// ConfigureSplitDNS routes <domain> and zenify.internal to the cluster DNS on
// the cni0 link only. A global resolved.conf.d drop-in is deliberately avoided:
// resolved merges every drop-in into one scope and breaks other local zones.
// The unit re-applies the per-link settings at boot; if cni0 is recreated while
// the host is up, re-running `zenify infra up` re-applies them.
func ConfigureSplitDNS(ctx context.Context, r Runner, domain string) (changed bool, err error) {
	content, err := dnsUnit(domain)
	if err != nil {
		return false, fmt.Errorf("split dns: %w", err)
	}
	cur, exists, err := readRootFile(ctx, r, dnsUnitPath)
	if err != nil {
		return false, fmt.Errorf("split dns: %w", err)
	}
	contentChanged := !exists || !bytes.Equal(cur, content)
	if contentChanged {
		if err := writeRootFile(ctx, r, dnsUnitPath, content); err != nil {
			return false, fmt.Errorf("split dns: %w", err)
		}
		changed = true
		if _, err := r.Run(ctx, nil, "sudo", "systemctl", "daemon-reload"); err != nil {
			return changed, fmt.Errorf("split dns: daemon-reload: %w", err)
		}
	} else if _, err := r.Run(ctx, nil, "sudo", "systemctl", "is-active", "--quiet", dnsUnitName); err != nil {
		// a positive exit code means inactive (stopped or failed earlier): start it again;
		// anything else (sudo or systemctl could not run) is a real error
		if exitCode(err) <= 0 {
			return false, fmt.Errorf("split dns: is-active: %w", err)
		}
		changed = true
	}
	if !changed {
		return false, nil
	}
	if contentChanged && exists {
		// RemainAfterExit keeps the old unit "active": a content change needs a restart
		if _, err := r.Run(ctx, nil, "sudo", "systemctl", "enable", dnsUnitName); err != nil {
			return changed, fmt.Errorf("split dns: enable: %w", err)
		}
		if _, err := r.Run(ctx, nil, "sudo", "systemctl", "restart", dnsUnitName); err != nil {
			return changed, fmt.Errorf("split dns: restart: %w", err)
		}
		return changed, nil
	}
	if _, err := r.Run(ctx, nil, "sudo", "systemctl", "enable", "--now", dnsUnitName); err != nil {
		return changed, fmt.Errorf("split dns: enable --now: %w", err)
	}
	return changed, nil
}

// RemoveTrustAndDNS undoes InstallCA and ConfigureSplitDNS for `down --purge`.
// Each step ignores only "already gone".
func RemoveTrustAndDNS(ctx context.Context, r Runner, home string) error {
	if _, exists, err := readRootFile(ctx, r, dnsUnitPath); err != nil {
		return fmt.Errorf("remove dns: %w", err)
	} else if exists {
		if _, err := r.Run(ctx, nil, "sudo", "systemctl", "disable", "--now", dnsUnitName); err != nil {
			return fmt.Errorf("remove dns: disable: %w", err)
		}
		if _, err := r.Run(ctx, nil, "sudo", "rm", "-f", dnsUnitPath); err != nil {
			return fmt.Errorf("remove dns: rm unit: %w", err)
		}
		if _, err := r.Run(ctx, nil, "sudo", "systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("remove dns: daemon-reload: %w", err)
		}
	}
	if _, err := r.Run(ctx, nil, "sudo", "resolvectl", "revert", "cni0"); err != nil &&
		!strings.Contains(err.Error(), "No such device") { // cni0 absent: nothing to revert
		return fmt.Errorf("remove dns: revert cni0: %w", err)
	}

	copyPath := filepath.Join(infraDir(home), caCopyName)
	installed, err := os.ReadFile(copyPath) //nolint:gosec // G304 -- the kit's own CA copy
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove CA: read %s: %w", copyPath, err)
	}
	_, exists, err := readRootFile(ctx, r, caSystemPath)
	if err != nil {
		return fmt.Errorf("remove CA: %w", err)
	}
	stale := false // the file is gone but an earlier failed run left it in the bundle
	if !exists && len(installed) > 0 {
		if stale, err = bundleHas(installed); err != nil {
			return fmt.Errorf("remove CA: %w", err)
		}
	}
	if exists {
		if _, err := r.Run(ctx, nil, "sudo", "rm", "-f", caSystemPath); err != nil {
			return fmt.Errorf("remove CA: rm: %w", err)
		}
	}
	if exists || stale {
		if _, err := r.Run(ctx, nil, "sudo", "update-ca-certificates"); err != nil {
			return fmt.Errorf("remove CA: update-ca-certificates: %w", err)
		}
	}

	if err := os.Remove(copyPath); err != nil && !errors.Is(err, os.ErrNotExist) { // already gone
		return fmt.Errorf("remove CA: rm %s: %w", copyPath, err)
	}
	if _, err := os.Stat(filepath.Join(nssDir(home), "cert9.db")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // no NSS db: nothing to remove
		}
		return fmt.Errorf("remove CA: stat NSS db: %w", err)
	}
	if _, err := r.Run(ctx, nil, "certutil", "-D", "-d", "sql:"+nssDir(home), "-n", caNSSNick); err != nil && !nssNotFound(err) {
		return certutilErr("remove NSS cert", err)
	}
	return nil
}

// RestartSplitDNS re-runs the split DNS unit: cni0 may have been recreated
// since it last ran, dropping its per-link settings.
func RestartSplitDNS(ctx context.Context, r Runner) error {
	if _, err := r.Run(ctx, nil, "sudo", "systemctl", "restart", dnsUnitName); err != nil {
		return fmt.Errorf("split dns: restart: %w", err)
	}
	return nil
}

// Host CA store paths, readable by any user; tests point them elsewhere.
var (
	caSystemReadPath = caSystemPath
	caBundlePath     = "/etc/ssl/certs/ca-certificates.crt"
)

// bundleHas reports whether the system CA bundle holds crt (whitespace ignored).
func bundleHas(crt []byte) (bool, error) {
	b, err := os.ReadFile(caBundlePath)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", caBundlePath, err)
	}
	block, _ := pem.Decode(crt)
	if block == nil {
		return false, errors.New("no PEM block")
	}
	body := bytes.Join(bytes.Fields(pem.EncodeToMemory(&pem.Block{Type: block.Type, Bytes: block.Bytes})), nil)
	body = bytes.TrimSuffix(bytes.TrimPrefix(body, []byte("-----BEGINCERTIFICATE-----")), []byte("-----ENDCERTIFICATE-----"))
	return bytes.Contains(bytes.Join(bytes.Fields(b), nil), body), nil
}

// CATrusted reports whether the CA the kit installed (~/.zenify/infra/ca.crt)
// is the one in the system store, in the generated system bundle, and in the
// user's NSS DB with the C trust flag. Any store holding another CA, or
// missing it, is false.
func CATrusted(ctx context.Context, r Runner, home string) (bool, error) {
	want, err := os.ReadFile(filepath.Join(infraDir(home), caCopyName)) //nolint:gosec // G304 -- the kit's own CA copy
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("ca trust: read CA copy: %w", err)
	}
	sys, err := os.ReadFile(caSystemReadPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("ca trust: read %s: %w", caSystemReadPath, err)
	}
	if !samePEM(sys, want) {
		return false, nil
	}
	if ok, err := bundleHas(want); err != nil || !ok {
		if err != nil {
			return false, fmt.Errorf("ca trust: %w", err)
		}
		return false, nil
	}
	if _, err := os.Stat(filepath.Join(nssDir(home), "cert9.db")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("ca trust: stat NSS db: %w", err)
	}
	db := "sql:" + nssDir(home)
	have, err := r.Run(ctx, nil, "certutil", "-L", "-d", db, "-n", caNSSNick, "-a")
	if err != nil {
		if nssNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("ca trust: certutil -L -n: %w", err)
	}
	if !samePEM(have, want) {
		return false, nil
	}
	list, err := r.Run(ctx, nil, "certutil", "-L", "-d", db)
	if err != nil {
		return false, fmt.Errorf("ca trust: certutil -L: %w", err)
	}
	return nssTrustsCA(list), nil
}

// nssTrustsCA finds the kit's nickname in `certutil -L` and checks its SSL
// trust attributes carry C (trusted CA).
func nssTrustsCA(list []byte) bool {
	for _, line := range strings.Split(string(list), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && strings.HasPrefix(line, caNSSNick+" ") {
			ssl, _, _ := strings.Cut(f[len(f)-1], ",")
			return strings.Contains(ssl, "C")
		}
	}
	return false
}

// DNSResolves runs a real `resolvectl query host` and reports whether it
// answers the ingress IP; a positive exit code means the name does not resolve.
// Another local resolver (e.g. a *.test wildcard to 127.0.0.1) does not count,
// and false also covers systemd-resolved not running (resolvectl exits non-zero).
func DNSResolves(ctx context.Context, r Runner, host string) (bool, error) {
	out, err := r.Run(ctx, nil, "resolvectl", "query", host)
	if err != nil {
		if exitCode(err) > 0 {
			return false, nil
		}
		return false, fmt.Errorf("resolvectl query %s: %w", host, err)
	}
	return bytes.Contains(out, []byte(" "+TraefikClusterIP)), nil
}
