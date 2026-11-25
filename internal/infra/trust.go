package infra

import (
	"bytes"
	"context"
	"encoding/base64"
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
	pem, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		return nil, fmt.Errorf("fetch CA: decode %s: %w", caSecretKey, err)
	}
	if !bytes.Contains(pem, []byte("BEGIN CERTIFICATE")) {
		return nil, fmt.Errorf("fetch CA: secret %s/%s has no certificate in %s", PlatformNamespace, caSecretName, caSecretKey)
	}
	return pem, nil
}

func samePEM(a, b []byte) bool {
	return bytes.Equal(bytes.Join(bytes.Fields(a), nil), bytes.Join(bytes.Fields(b), nil))
}

// InstallCA makes the host and the user's NSS database (Chrome, Firefox) trust
// the CA, and keeps a copy at ~/.zenify/infra/ca.crt. changed is false when
// everything already held this exact CA.
func InstallCA(ctx context.Context, r Runner, home string, pem []byte) (changed bool, err error) {
	if !bytes.Contains(pem, []byte("BEGIN CERTIFICATE")) {
		return false, errors.New("install CA: empty or invalid PEM")
	}

	cur, exists, err := readRootFile(ctx, r, caSystemPath)
	if err != nil {
		return false, fmt.Errorf("install CA: %w", err)
	}
	if !exists || !samePEM(cur, pem) {
		if err := writeRootFile(ctx, r, caSystemPath, pem); err != nil {
			return false, fmt.Errorf("install CA: %w", err)
		}
		if _, err := r.Run(ctx, nil, "sudo", "update-ca-certificates"); err != nil {
			return true, fmt.Errorf("install CA: update-ca-certificates: %w", err)
		}
		changed = true
	}

	nssChanged, err := installNSS(ctx, r, home, pem)
	if err != nil {
		return changed, err
	}

	copied, err := saveCACopy(home, pem)
	if err != nil {
		return changed || nssChanged, err
	}
	return changed || nssChanged || copied, nil
}

// saveCACopy keeps a world-readable copy at ~/.zenify/infra/ca.crt.
func saveCACopy(home string, pem []byte) (bool, error) {
	copyPath := filepath.Join(infraDir(home), caCopyName)
	have, err := os.ReadFile(copyPath) //nolint:gosec // G304 -- path under the user home, built from fixed names
	if err == nil && samePEM(have, pem) {
		return false, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("install CA: read %s: %w", copyPath, err)
	}
	if err := writeFileAtomic(home, caCopyName, pem); err != nil {
		return false, fmt.Errorf("install CA: %w", err)
	}
	if err := os.Chmod(copyPath, 0o644); err != nil { //nolint:gosec // G302 -- a public CA certificate, meant to be world-readable
		return true, fmt.Errorf("install CA: chmod %s: %w", copyPath, err)
	}
	return true, nil
}

func nssDir(home string) string { return filepath.Join(home, ".pki", "nssdb") }

// installNSS adds the CA to the user's NSS DB, creating the DB when absent.
func installNSS(ctx context.Context, r Runner, home string, pem []byte) (bool, error) {
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
	case err == nil && samePEM(have, pem):
		return false, nil
	case err == nil:
		// a different CA under our nickname (the CA was regenerated): replace it
		if _, err := r.Run(ctx, nil, "certutil", "-D", "-d", db, "-n", caNSSNick); err != nil {
			return false, certutilErr("delete stale NSS cert", err)
		}
	case !nssNotFound(err):
		return false, certutilErr("list NSS certs", err)
	}
	if _, err := r.Run(ctx, bytes.NewReader(pem), "certutil", "-A", "-d", db, "-t", "C,,", "-n", caNSSNick); err != nil {
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
	if !domainRe.MatchString(domain) {
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
		// exit 3 = inactive (failed earlier or stopped): start it again
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
func RemoveTrustAndDNS(ctx context.Context, r Runner, _ string) error {
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

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("remove CA: home dir: %w", err)
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
