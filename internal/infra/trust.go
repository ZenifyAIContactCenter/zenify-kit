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
	if !exists || !samePEM(cur, crt) {
		if err := writeRootFile(ctx, r, caSystemPath, crt); err != nil {
			return false, fmt.Errorf("install CA: %w", err)
		}
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
	if len(domain) > 253 || !domainRe.MatchString(domain) {
		return nil, fmt.Errorf("invalid domain %q", domain)
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

	if _, exists, err := readRootFile(ctx, r, caSystemPath); err != nil {
		return fmt.Errorf("remove CA: %w", err)
	} else if exists {
		if _, err := r.Run(ctx, nil, "sudo", "rm", "-f", caSystemPath); err != nil {
			return fmt.Errorf("remove CA: rm: %w", err)
		}
		if _, err := r.Run(ctx, nil, "sudo", "update-ca-certificates"); err != nil {
			return fmt.Errorf("remove CA: update-ca-certificates: %w", err)
		}
	}

	copyPath := filepath.Join(infraDir(home), caCopyName)
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
