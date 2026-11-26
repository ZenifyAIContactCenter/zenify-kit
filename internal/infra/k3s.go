package infra

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"os/exec"
	"os/user"
	"slices"
	"strings"
	"text/template"
	"time"
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
	// restartMarker exists while a k3s config change awaits a successful installer run.
	restartMarker = "/etc/rancher/k3s/.zenify-restart-pending"
	// fwMarker exists while a firewall file change awaits a successful reload.
	fwMarker = "/etc/nftables.d/.zenify-firewall-reload-pending"
	// mirrorHostsPath is containerd's config for the registry alias, rendered by
	// k3s from registries.yaml (containerd config_path = .../containerd/certs.d).
	mirrorHostsPath = "/var/lib/rancher/k3s/agent/etc/containerd/certs.d/" + RegistryHost + "/hosts.toml"
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
	applyK3s             // k3s reads it only at start: restart k3s
	applyFirewall        // reload the firewall unit
)

type hostFile struct {
	path, asset string
	apply       int
}

// kubeconfigGroup is the primary group of the user running the kit: k3s
// writes its admin kubeconfig 0640 to that group so kubectl works without
// making the file world-readable.
var kubeconfigGroup = func() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("current user: %w", err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		return "", fmt.Errorf("primary group of %s: %w", u.Username, err)
	}
	return g.Name, nil
}

func (f hostFile) render() ([]byte, error) {
	group, err := kubeconfigGroup()
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", f.asset, err)
	}
	return renderAsset(f.asset, map[string]string{
		"KubeconfigGroup":   group,
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

// nodeReadyTimeout bounds the wait for the node after a k3s restart.
var nodeReadyTimeout = 3 * time.Minute

// InstallK3s writes the k3s config, registry mirror, Traefik HelmChartConfig and
// the API/kubelet firewall, runs the pinned k3s installer when k3s is not at the
// pinned version, and restarts k3s when its config or registry mirror changed
// or containerd has not loaded the mirror. A restart marker written before such
// a change and removed only after the restart succeeded makes a run after a
// failed restart retry it.
func InstallK3s(ctx context.Context, r Runner) (changed bool, err error) {
	_, pending, err := readRootFile(ctx, r, restartMarker)
	if err != nil {
		return false, fmt.Errorf("install k3s: %w", err)
	}
	// a run that wrote firewall files but failed before the reload left the
	// old rules active while the files already match
	_, fwDirty, err := readRootFile(ctx, r, fwMarker)
	if err != nil {
		return false, fmt.Errorf("install k3s: %w", err)
	}
	fwPending := fwDirty
	for _, f := range k3sHostFiles() {
		content, err := f.render()
		if err != nil {
			return changed, fmt.Errorf("install k3s: %w", err)
		}
		cur, exists, err := readRootFile(ctx, r, f.path)
		if err != nil {
			return changed, fmt.Errorf("install k3s: %w", err)
		}
		if exists && bytes.Equal(cur, content) {
			continue
		}
		if f.apply == applyK3s && !pending {
			if err := writeRootFile(ctx, r, restartMarker, []byte("k3s restart pending\n")); err != nil {
				return changed, fmt.Errorf("install k3s: restart marker: %w", err)
			}
			pending = true
		}
		if f.apply == applyFirewall && !fwPending {
			if err := writeRootFile(ctx, r, fwMarker, []byte("firewall reload pending\n")); err != nil {
				return changed, fmt.Errorf("install k3s: firewall marker: %w", err)
			}
			fwPending = true
		}
		if err := writeRootFile(ctx, r, f.path, content); err != nil {
			return changed, fmt.Errorf("install k3s: %w", err)
		}
		changed = true
		fwDirty = fwDirty || f.apply == applyFirewall
	}

	if !fwDirty {
		// a failed restart in an earlier run leaves the unit inactive
		if _, err := r.Run(ctx, nil, "sudo", "systemctl", "is-active", "--quiet", firewallUnit); err != nil {
			if exitCode(err) <= 0 {
				return changed, fmt.Errorf("install k3s: firewall: is-active: %w", err)
			}
			fwDirty = true
		}
	}
	if fwDirty {
		changed = true
		for _, args := range [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", firewallUnit},
			{"systemctl", "restart", firewallUnit},
		} {
			if _, err := r.Run(ctx, nil, "sudo", args...); err != nil {
				return changed, fmt.Errorf("install k3s: firewall: %v: %w", args, err)
			}
		}
		if fwPending {
			if _, err := r.Run(ctx, nil, "sudo", "rm", "-f", fwMarker); err != nil {
				return changed, fmt.Errorf("install k3s: remove firewall marker: %w", err)
			}
		}
		// k3s.service requires the firewall unit, so restarting it restarts k3s
		if v, err := installedK3s(ctx, r); err != nil {
			return changed, fmt.Errorf("install k3s: %w", err)
		} else if v != "" {
			if err := waitNodeReady(ctx, r); err != nil {
				return changed, fmt.Errorf("install k3s: after firewall restart: %w", err)
			}
		}
	}

	ver, err := installedK3s(ctx, r)
	if err != nil {
		return changed, fmt.Errorf("install k3s: %w", err)
	}
	if ver != "" && !pending {
		// files can match while k3s still runs an older config (e.g. written by
		// a run whose restart never happened): check what containerd loaded
		loaded, err := mirrorLoaded(ctx, r)
		if err != nil {
			return changed, fmt.Errorf("install k3s: %w", err)
		}
		pending = !loaded
	}
	if ver == K3sVersion && !pending {
		return changed, nil
	}
	if ver != K3sVersion {
		script, err := r.Run(ctx, nil, "curl", "-sfL", k3sInstaller)
		if err != nil {
			return changed, fmt.Errorf("install k3s: download installer: %w", err)
		}
		if _, err := r.Run(ctx, bytes.NewReader(script), "sudo", "env", "INSTALL_K3S_VERSION="+K3sVersion, "sh", "-s", "-"); err != nil {
			return true, fmt.Errorf("install k3s: run installer %s: %w", K3sVersion, err)
		}
	}
	// the installer skips the service restart when the binary and unit are
	// unchanged, so config.yaml/registries.yaml changes need an explicit one
	if _, err := r.Run(ctx, nil, "sudo", "systemctl", "restart", "k3s"); err != nil {
		return true, fmt.Errorf("install k3s: restart k3s: %w", err)
	}
	if err := waitNodeReady(ctx, r); err != nil {
		return true, fmt.Errorf("install k3s: %w", err)
	}
	loaded, err := mirrorLoaded(ctx, r)
	if err != nil {
		return true, fmt.Errorf("install k3s: %w", err)
	}
	if !loaded {
		return true, fmt.Errorf("install k3s: k3s restarted but %s does not route %s to %s", mirrorHostsPath, RegistryHost, mirrorEndpoint())
	}
	if _, err := r.Run(ctx, nil, "sudo", "rm", "-f", restartMarker); err != nil {
		return true, fmt.Errorf("install k3s: remove restart marker: %w", err)
	}
	return true, nil
}

func mirrorEndpoint() string { return "http://" + RegistryClusterIP + ":5000" }

// mirrorLoaded reports whether k3s rendered the registry mirror into
// containerd's hosts.toml, which k3s writes only at start.
func mirrorLoaded(ctx context.Context, r Runner) (bool, error) {
	b, exists, err := readRootFile(ctx, r, mirrorHostsPath)
	if err != nil {
		return false, err
	}
	return exists && bytes.Contains(b, []byte(mirrorEndpoint())), nil
}

// waitNodeReady polls until every node reports Ready; read errors are expected
// while the API server restarts.
func waitNodeReady(ctx context.Context, r Runner) error {
	ctx, cancel := context.WithTimeout(ctx, nodeReadyTimeout)
	defer cancel()
	var last string
	for {
		out, err := kubectl(ctx, r, nil, "get", "nodes", "-o",
			`jsonpath={range .items[*]}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}`)
		if err != nil {
			last = err.Error()
		} else {
			st := strings.Fields(string(out))
			if len(st) > 0 && !slices.ContainsFunc(st, func(s string) bool { return s != "True" }) {
				return nil
			}
			last = "node Ready statuses " + strings.Join(st, ",")
		}
		if err := sleepCtx(ctx, pollInterval); err != nil {
			return fmt.Errorf("wait node ready: not ready within %s (last: %s)", nodeReadyTimeout, last)
		}
	}
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
