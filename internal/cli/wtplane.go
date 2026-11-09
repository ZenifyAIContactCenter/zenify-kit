package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// zenSlugRe picks the ZEN work item number out of a `wt new` slug.
var zenSlugRe = regexp.MustCompile(`^zen-(\d+)`)

// planeStartHook is wt.NewOptions.OnStart: for a zen-<n> slug it moves ZEN-<n>
// on Plane to In Progress with the developer's PAT (spec workflow-state-notify
// §2G). The returned string is a warning; "" means done or nothing to do.
func planeStartHook(getenv func(string) string, ws string, client *http.Client) func(slug string) string {
	return func(slug string) string {
		m := zenSlugRe.FindStringSubmatch(slug)
		if m == nil {
			return ""
		}
		key := "ZEN-" + m[1]
		pat, err := planePAT(getenv, ws)
		if err != nil || pat == "" {
			return fmt.Sprintf("plane: no %s, %s not moved to In Progress (run zenify up --apply to add it)", planePATKey, key)
		}
		man, err := brokerManifest("", "")
		if err != nil || man.Plane == nil {
			return fmt.Sprintf("plane: no Plane in the manifest, %s not moved", key)
		}
		p := planeAPI{client: client, base: strings.TrimRight(man.Plane.URL, "/") + "/api/v1", pat: pat}
		w, err := startZenItem(p, man.Plane.Workspace, key)
		if err != nil {
			return fmt.Sprintf("plane: %s not moved to In Progress: %v", key, err)
		}
		return w
	}
}

type planeState struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Group  string `json:"group"`
	Triage bool   `json:"is_triage"`
}

// startZenItem applies the §2G rules to one work item. The holder comes from
// the state, not the assignees (they carry watchers): Backlog = nobody holds
// it, Todo = held, by the dev iff the dev is an assignee.
func startZenItem(p planeAPI, slug, key string) (string, error) {
	var me struct {
		ID string `json:"id"`
	}
	if err := p.do(http.MethodGet, "/users/me/", nil, &me); err != nil {
		return "", err
	}
	var item struct {
		ID        string   `json:"id"`
		Project   string   `json:"project"`
		State     string   `json:"state"`
		Assignees []string `json:"assignees"`
	}
	if err := p.do(http.MethodGet, "/workspaces/"+slug+"/work-items/"+key+"/", nil, &item); err != nil {
		return "", err
	}
	var states struct {
		Results []planeState `json:"results"`
	}
	proj := "/workspaces/" + slug + "/projects/" + item.Project
	if err := p.do(http.MethodGet, proj+"/states/", nil, &states); err != nil {
		return "", err
	}
	var cur, target *planeState
	for i := range states.Results {
		s := &states.Results[i]
		if s.ID == item.State {
			cur = s
		}
		if s.Name == "In Progress" {
			target = s
		}
	}
	// Intake items sit in Plane's triage state, which the list leaves out.
	if cur == nil || cur.Triage {
		return fmt.Sprintf("plane: %s is in Intake, not moved", key), nil
	}
	dev := slices.Contains(item.Assignees, me.ID)
	body := map[string]any{}
	switch cur.Group {
	case "started":
		return "", nil
	case "completed", "cancelled":
		return fmt.Sprintf("plane: %s is %s, not moved", key, cur.Name), nil
	case "unstarted":
		if !dev {
			return fmt.Sprintf("plane: %s is held by someone else (%s), not moved", key, cur.Name), nil
		}
	case "backlog":
		if !dev {
			body["assignees"] = append(slices.Clone(item.Assignees), me.ID)
		}
	default:
		return fmt.Sprintf("plane: %s has unknown state group %q, not moved", key, cur.Group), nil
	}
	if target == nil {
		return fmt.Sprintf("plane: project has no In Progress state, %s not moved", key), nil
	}
	body["state"] = target.ID
	if err := p.do(http.MethodPatch, proj+"/work-items/"+item.ID+"/", body, nil); err != nil {
		return "", err
	}
	return "", nil
}

type planeAPI struct {
	client *http.Client
	base   string
	pat    string
}

// do sends one Plane REST call with the PAT in X-API-Key. Errors name the
// status only, never the token.
func (p planeAPI) do(method, path string, in, out any) error {
	var rd io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, p.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", p.pat)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach Plane: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("plane answered %s for %s %s", resp.Status, method, path)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}
