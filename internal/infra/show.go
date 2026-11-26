package infra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// SeedAccount is a seeded login; it deliberately has no password field.
type SeedAccount struct {
	Role   string `json:"role"`
	Email  string `json:"email"`
	Tenant string `json:"tenant"`
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
// read from the child Applications' values, plus the seed accounts. It only
// reads (kubectl get). An unknown slug is an error naming the envs.
func Show(ctx context.Context, r Runner, home, domain, slug string) (ShowReport, error) {
	rep := ShowReport{Domain: domain, CA: CACopyPath(home), Envs: []ShowEnv{}, Accounts: SeedAccounts}
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
		envDomain := root.envDomain()
		if envDomain == "" {
			envDomain = domain
		}
		e := ShowEnv{Slug: s, Sync: root.Status.Sync.Status, Health: root.Status.Health.Status, Apps: []ShowApp{}}
		for _, a := range envApps {
			if a.svc == "" {
				continue
			}
			name := ChildAppName(a.name, s)
			tag, err := byName[name].imageTag() // absent child: empty source, no tag
			if err != nil {
				return rep, fmt.Errorf("show: application %s: image tag unreadable: %w", name, err)
			}
			e.Apps = append(e.Apps, ShowApp{Svc: a.svc, URL: "https://" + Host(a.svc, s, envDomain), Repo: a.name, Tag: tag})
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
