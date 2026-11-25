package infra

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"os/exec"
	"text/template"
)

//go:embed assets/*.tmpl
var assets embed.FS

const (
	traefikConfigPath = "/var/lib/rancher/k3s/server/manifests/traefik-config.yaml"
	registriesPath    = "/etc/rancher/k3s/registries.yaml"
	// nftables.service (Ubuntu) flushes the whole ruleset from /etc/nftables.conf,
	// which would wipe docker's and k3s's rules; a dedicated table loaded by its
	// own oneshot unit leaves every other table alone.
	nftPath      = "/etc/nftables.d/zenify-k3s.nft"
	nftTable     = "zenify_k3s"
	firewallUnit = "zenify-k3s-firewall.service"
	unitPath     = "/etc/systemd/system/" + firewallUnit
	k3sInstaller = "https://get.k3s.io"
)

// renderAsset executes the embedded template assets/<name> with data.
func renderAsset(name string, data any) ([]byte, error) {
	t, err := template.New(name).Option("missingkey=error").ParseFS(assets, "assets/"+name)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// What a changed host file needs to take effect.
const (
	applyLive     = iota // k3s's deploy controller watches the manifests dir
	applyK3s             // k3s reads it only at start: rerun the installer (restarts k3s)
	applyFirewall        // reload the firewall unit
)

type hostFile struct {
	path, asset string
	apply       int
}

func (f hostFile) render() ([]byte, error) {
	return renderAsset(f.asset, map[string]string{
		"TraefikClusterIP":  TraefikClusterIP,
		"RegistryHost":      RegistryHost,
		"RegistryClusterIP": RegistryClusterIP,
		"Table":             nftTable,
		"NftPath":           nftPath,
	})
}

func k3sHostFiles() []hostFile {
	return []hostFile{
		{path: k3sConfigPath, asset: "k3s-config.yaml.tmpl", apply: applyK3s},
		{path: registriesPath, asset: "registries.yaml.tmpl", apply: applyK3s},
		{path: traefikConfigPath, asset: "traefik-config.yaml.tmpl", apply: applyLive},
		{path: nftPath, asset: "k3s-firewall.nft.tmpl", apply: applyFirewall},
		{path: unitPath, asset: "k3s-firewall.service.tmpl", apply: applyFirewall},
	}
}

// InstallK3s writes the k3s config, registry mirror, Traefik HelmChartConfig and
// the API/kubelet firewall, then runs the pinned k3s installer when k3s is not
// at the pinned version or its config or registry mirror changed.
func InstallK3s(ctx context.Context, r Runner) (changed bool, err error) {
	var k3sDirty, fwDirty bool
	for _, f := range k3sHostFiles() {
		content, err := f.render()
		if err != nil {
			return changed, fmt.Errorf("install k3s: %w", err)
		}
		c, err := ensureRootFile(ctx, r, f.path, content)
		if err != nil {
			return changed, fmt.Errorf("install k3s: %w", err)
		}
		if c {
			changed = true
			k3sDirty = k3sDirty || f.apply == applyK3s
			fwDirty = fwDirty || f.apply == applyFirewall
		}
	}

	if fwDirty {
		for _, args := range [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", firewallUnit},
			{"systemctl", "restart", firewallUnit},
		} {
			if _, err := r.Run(ctx, nil, "sudo", args...); err != nil {
				return changed, fmt.Errorf("install k3s: firewall: %v: %w", args, err)
			}
		}
	}

	ver, err := installedK3s(ctx, r)
	if err != nil {
		return changed, fmt.Errorf("install k3s: %w", err)
	}
	if ver == K3sVersion && !k3sDirty {
		return changed, nil
	}
	script, err := r.Run(ctx, nil, "curl", "-sfL", k3sInstaller)
	if err != nil {
		return changed, fmt.Errorf("install k3s: download installer: %w", err)
	}
	if _, err := r.Run(ctx, bytes.NewReader(script), "sudo", "env", "INSTALL_K3S_VERSION="+K3sVersion, "sh", "-s", "-"); err != nil {
		return true, fmt.Errorf("install k3s: run installer %s: %w", K3sVersion, err)
	}
	return true, nil
}

// installedK3s returns the installed k3s version, or "" when k3s is absent.
func installedK3s(ctx context.Context, r Runner) (string, error) {
	out, err := r.Run(ctx, nil, "k3s", "--version")
	if errors.Is(err, exec.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("k3s --version: %w", err)
	}
	return parseK3sVersion(out)
}
