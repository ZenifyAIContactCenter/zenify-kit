package infra

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// SeedAccount is a seeded login. Password is filled from seed.password by
// Workspace.Accounts, never read from an account entry.
type SeedAccount struct {
	Role     string `json:"role"`
	Email    string `json:"email"`
	Tenant   string `json:"tenant"`
	Password string `json:"password,omitempty"`
}

// ShowApp is one app of an env: its URL, repo and running image tag.
type ShowApp struct {
	Svc  string `json:"svc"`
	URL  string `json:"url"`
	Repo string `json:"repo"`
	Tag  string `json:"tag"`
}

// ShowEnv is one env with its apps.
type ShowEnv struct {
	Slug   string    `json:"slug"`
	Sync   string    `json:"sync"`
	Health string    `json:"health"`
	Apps   []ShowApp `json:"apps"`
}

// ShowReport is what `zenify infra show --local` prints: how to use the envs.
type ShowReport struct {
	Domain   string        `json:"domain"`
	CA       string        `json:"ca"`
	Envs     []ShowEnv     `json:"envs"`
	Accounts []SeedAccount `json:"accounts"`
	// LoginHelp says where the seed password is documented, when seed.password is unset.
	LoginHelp string      `json:"loginHelp,omitempty"`
	Cluster   ShowCluster `json:"cluster"`
}

// ShowNode is one cluster node and its k3s version.
type ShowNode struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ShowRegistry is the in-cluster registry: the alias pods pull with and the
// ClusterIP address the host reaches it on.
type ShowRegistry struct {
	Alias   string `json:"alias"`
	Address string `json:"address"`
	Push    string `json:"push"`
	Pull    string `json:"pull"`
	List    string `json:"list"`
}

// ShowArgoCD says how to reach ArgoCD and its initial admin password, empty
// once the dev deleted argocd-initial-admin-secret (PasswordCommand stays).
type ShowArgoCD struct {
	URL             string `json:"url"`
	Password        string `json:"password,omitempty"`
	PasswordCommand string `json:"passwordCommand"`
	IngressURL      string `json:"ingressUrl,omitempty"`
}

// ShowCluster is how a dev reaches the local cluster with their own tools.
type ShowCluster struct {
	Kubeconfig     string       `json:"kubeconfig"`
	Export         string       `json:"export"`
	KubeconfigNote string       `json:"kubeconfigNote"`
	Context        string       `json:"context,omitempty"`
	Nodes          []ShowNode   `json:"nodes"`
	Registry       ShowRegistry `json:"registry"`
	ArgoCD         *ShowArgoCD  `json:"argocd,omitempty"`
	// OpenBao is the in-cluster address, set only when an OpenBao Service exists.
	OpenBao string `json:"openbao,omitempty"`
}

// argoPasswordCommand reads the initial admin password by hand.
const argoPasswordCommand = `kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d` //nolint:gosec // G101 -- a command the dev runs, not a credential

func baseCluster() (ShowCluster, error) {
	group, err := kubeconfigGroup()
	if err != nil {
		return ShowCluster{}, err
	}
	addr := RegistryClusterIP + ":5000"
	return ShowCluster{
		Kubeconfig:     K3sKubeconfig,
		Export:         "export KUBECONFIG=" + K3sKubeconfig,
		KubeconfigNote: "mode 0640, readable by group " + group + ", the devbox user's primary group",
		Nodes:          []ShowNode{},
		Registry: ShowRegistry{
			Alias:   RegistryHost,
			Address: addr,
			Push:    "docker tag <image> " + addr + "/<name>:<tag> && docker push " + addr + "/<name>:<tag>",
			Pull:    "image: " + RegistryHost + "/<name>:<tag>",
			List:    "curl http://" + addr + "/v2/_catalog",
		},
	}, nil
}

// clusterLive fills the context, nodes, ArgoCD and OpenBao rows (kubectl get only).
func clusterLive(ctx context.Context, r Runner, c *ShowCluster, domain string, argo bool) error {
	out, err := kubectl(ctx, r, nil, "config", "current-context")
	if err != nil {
		return fmt.Errorf("read kubeconfig context: %w", err)
	}
	c.Context = strings.TrimSpace(string(out))
	out, err = kubectl(ctx, r, nil, "get", "nodes", "-o",
		`jsonpath={range .items[*]}{.metadata.name}{" "}{.status.nodeInfo.kubeletVersion}{"\n"}{end}`)
	if err != nil {
		return fmt.Errorf("read nodes: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if f := strings.Fields(line); len(f) == 2 {
			c.Nodes = append(c.Nodes, ShowNode{Name: f[0], Version: f[1]})
		}
	}
	if argo {
		a := &ShowArgoCD{URL: "https://argocd-server." + argoNS + ".svc", PasswordCommand: argoPasswordCommand}
		out, err = kubectl(ctx, r, nil, "-n", argoNS, "get", "ingress", "-o", `jsonpath={.items[*].spec.rules[*].host}`)
		if err != nil {
			return fmt.Errorf("read argocd ingress: %w", err)
		}
		if host := "argocd." + domain; slices.Contains(strings.Fields(string(out)), host) {
			a.IngressURL = "https://" + host
		}
		out, err = kubectl(ctx, r, nil, "-n", argoNS, "get", "secret", "argocd-initial-admin-secret",
			"--ignore-not-found", "-o", "jsonpath={.data.password}")
		if err != nil {
			return fmt.Errorf("read argocd admin password: %w", err)
		}
		if enc := strings.TrimSpace(string(out)); enc != "" {
			pw, err := base64.StdEncoding.DecodeString(enc)
			if err != nil {
				return fmt.Errorf("decode argocd admin password: %w", err)
			}
			a.Password = string(pw)
		}
		c.ArgoCD = a
	}
	out, err = kubectl(ctx, r, nil, "get", "services", "-A", "-l", "app.kubernetes.io/name=openbao", "-o",
		`jsonpath={range .items[*]}{.metadata.name}{" "}{.metadata.namespace}{" "}{.spec.ports[0].port}{"\n"}{end}`)
	if err != nil {
		return fmt.Errorf("read openbao service: %w", err)
	}
	if f := strings.Fields(strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]); len(f) == 3 {
		c.OpenBao = "http://" + f[0] + "." + f[1] + ".svc:" + f[2]
	}
	return nil
}

// CACopyPath is the CA copy InstallCA keeps for tools (NODE_EXTRA_CA_CERTS).
func CACopyPath(home string) string { return filepath.Join(infraDir(home), caCopyName) }

// imageTag is image.tag of a child Application's helm values; an empty source
// or no image.tag is a legitimate "no tag". A source that does not parse, or a
// multi-source Application the kit never renders, is an error.
func (a appStatus) imageTag() (string, error) {
	if len(a.Spec.Source) == 0 {
		if len(a.Spec.Sources) > 0 && string(a.Spec.Sources) != "null" {
			return "", errors.New("spec.sources (multi-source) is not supported, want spec.source")
		}
		return "", nil
	}
	var src struct {
		Helm struct {
			ValuesObject struct {
				Image struct {
					Tag string `json:"tag"`
				} `json:"image"`
			} `json:"valuesObject"`
		} `json:"helm"`
	}
	if err := json.Unmarshal(a.Spec.Source, &src); err != nil {
		return "", fmt.Errorf("parse spec.source: %w", err)
	}
	return src.Helm.ValuesObject.Image.Tag, nil
}

// Show lists each env (or only slug) with its app URLs, repos and image tags
// read from the child Applications' values, the seed accounts and how to
// reach the cluster (kubeconfig, registry, ArgoCD, OpenBao). It only
// reads (kubectl get). An unknown slug is an error naming the envs.
func Show(ctx context.Context, r Runner, w Workspace, home, domain, slug string) (ShowReport, error) {
	rep := ShowReport{Domain: domain, CA: CACopyPath(home), Envs: []ShowEnv{}, Accounts: w.Accounts()}
	if w.Seed.Password == "" {
		rep.LoginHelp = w.Seed.PasswordHint
	}
	var err error
	if rep.Cluster, err = baseCluster(); err != nil {
		return rep, fmt.Errorf("show: %w", err)
	}
	var apps appList
	k3s, err := installedK3s(ctx, r)
	if err != nil {
		return rep, fmt.Errorf("show: %w", err)
	}
	if k3s != "" {
		argo, err := ArgoInstalledVersion(ctx, r)
		if err != nil {
			return rep, fmt.Errorf("show: %w", err)
		}
		if err := clusterLive(ctx, r, &rep.Cluster, domain, argo != ""); err != nil {
			return rep, fmt.Errorf("show: %w", err)
		}
		if argo != "" {
			if apps, err = listApps(ctx, r); err != nil {
				return rep, fmt.Errorf("show: %w", err)
			}
		}
	}
	byName := map[string]appStatus{}
	var slugs []string
	for _, it := range apps.Items {
		byName[it.Metadata.Name] = it.appStatus
		if s, ok := strings.CutPrefix(it.Metadata.Name, "env-"); ok {
			slugs = append(slugs, s)
		}
	}
	for _, s := range slugs {
		if slug != "" && s != slug {
			continue
		}
		root := byName[EnvAppName(s)]
		envDomain, err := root.envDomain(w.seedApp().Key, domain)
		if err != nil {
			return rep, fmt.Errorf("show: application %s: env domain unreadable: %w", EnvAppName(s), err)
		}
		e := ShowEnv{Slug: s, Sync: root.Status.Sync.Status, Health: root.Status.Health.Status, Apps: []ShowApp{}}
		for _, a := range w.svcApps() {
			name := ChildAppName(a.Key, s)
			tag, err := byName[name].imageTag() // absent child: empty source, no tag
			if err != nil {
				return rep, fmt.Errorf("show: application %s: image tag unreadable: %w", name, err)
			}
			e.Apps = append(e.Apps, ShowApp{Svc: a.Svc, URL: "https://" + Host(a.Svc, s, envDomain), Repo: a.Repo, Tag: tag})
		}
		rep.Envs = append(rep.Envs, e)
	}
	if slug != "" && len(rep.Envs) == 0 {
		list := "none"
		if len(slugs) > 0 {
			list = strings.Join(slugs, ", ")
		}
		return rep, fmt.Errorf("show: env %q not found (envs: %s)", slug, list)
	}
	return rep, nil
}
