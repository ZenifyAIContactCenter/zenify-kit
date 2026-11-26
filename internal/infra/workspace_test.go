package infra

import (
	"strings"
	"testing"
)

// infra.json is team-shared input: every value that reaches a token, a path or
// a command line is checked, and the error names the field and the rule.
func TestWorkspaceValidateRejectsUnsafeValues(t *testing.T) {
	long := strings.Repeat("a", maxLabelLen+1)
	ok := strings.Repeat("a", maxLabelLen)
	cases := []struct {
		name string
		mod  func(w *Workspace)
		want string // substring of the error; "" = valid
	}{
		{"fixture is valid", func(*Workspace) {}, ""},
		// a dev must learn the seed login somewhere: the password itself or where it is documented
		{"seed password without hint is valid", func(w *Workspace) { w.Seed.Password, w.Seed.PasswordHint = "pw", "" }, ""},
		{"seed without password or hint", func(w *Workspace) { w.Seed.Password, w.Seed.PasswordHint = "", "" }, "seed.password or seed.passwordHint"},
		{"30-byte svc and key are valid", func(w *Workspace) { w.EnvApps[4].Svc, w.EnvApps[4].Key = ok, ok }, ""},
		// the user's gh token is sent to helmRepoURL
		{"helm http", func(w *Workspace) { w.HelmRepoURL = "http://github.com/a/b.git" }, "helmRepoURL (an https://github.com"},
		{"helm other host", func(w *Workspace) { w.HelmRepoURL = "https://evil.example/a/b.git" }, "helmRepoURL"},
		{"helm userinfo", func(w *Workspace) { w.HelmRepoURL = "https://u:p@github.com/a/b.git" }, "without credentials"},
		{"helm no path", func(w *Workspace) { w.HelmRepoURL = "https://github.com" }, "helmRepoURL"},
		// repo reaches filepath.Join(ws, "repos", repo) and os.RemoveAll
		{"repo traversal", func(w *Workspace) { w.Builds[1].Repo = "..x" }, "builds[1].repo (^[a-z0-9][a-z0-9._-]*$, no ..)"},
		{"repo dotdot", func(w *Workspace) { w.Builds[1].Repo = "a..b" }, "builds[1].repo"},
		{"repo slash", func(w *Workspace) { w.Builds[1].Repo = "a/b" }, "builds[1].repo"},
		{"repo upper", func(w *Workspace) { w.Builds[1].Repo = "Shop" }, "builds[1].repo"},
		{"repo duplicate", func(w *Workspace) { w.Builds[2].Repo = w.Builds[1].Repo }, "builds[2].repo (duplicate)"},
		{"context absolute", func(w *Workspace) { w.Builds[0].Context = "/etc" }, "builds[0].context (a relative path inside the repo)"},
		{"context escapes", func(w *Workspace) { w.Builds[0].Context = "../other" }, "builds[0].context"},
		{"dockerfile escapes", func(w *Workspace) { w.Builds[0].Dockerfile = "../Dockerfile" }, "builds[0].dockerfile"},
		{"image charset", func(w *Workspace) { w.Builds[1].Image = "Shop API" }, "builds[1].image (registry path"},
		{"image empty segment", func(w *Workspace) { w.Builds[1].Image = "a//b" }, "builds[1].image"},
		{"image traversal", func(w *Workspace) { w.Builds[1].Image = "../x" }, "builds[1].image"},
		{"tag charset", func(w *Workspace) { w.Builds[0].Tag = "bad tag" }, "builds[0].tag"},
		{"secret charset", func(w *Workspace) { w.Builds[1].Secrets = []string{"NPM-TOKEN"} }, `builds[1].secrets/optionalSecrets "NPM-TOKEN" (^[a-z0-9_]+$)`},
		{"optional secret charset", func(w *Workspace) { w.Builds[2].OptionalSecrets = []string{"a b"} }, `builds[2].secrets/optionalSecrets "a b"`},
		{"key duplicate", func(w *Workspace) { w.EnvApps[1].Key = w.EnvApps[0].Key }, "envApps[1].key (duplicate)"},
		{"key not a label", func(w *Workspace) { w.EnvApps[0].Key = "Bad_Key" }, "envApps[0].key (DNS label of at most 30 bytes"},
		{"key too long", func(w *Workspace) { w.EnvApps[0].Key = long }, "envApps[0].key"},
		{"key env- prefix", func(w *Workspace) { w.EnvApps[0].Key = "env-x" }, "not starting with env-"},
		{"svc not a label", func(w *Workspace) { w.EnvApps[4].Svc = "be.x" }, "envApps[4].svc (DNS label of at most 30 bytes)"},
		{"svc too long", func(w *Workspace) { w.EnvApps[4].Svc = long }, "envApps[4].svc"},
		{"ingress path", func(w *Workspace) { w.EnvApps[4].Ingress[0].Path = "api" }, "envApps[4].ingress[0].values/path (path starts with /)"},
		// round 2: a fixed tag is reused as is, so per-env args would serve another env's image
		{"fixed tag with envArgs", func(w *Workspace) { w.Builds[3].Tag = "preview" }, "builds[3].tag (a fixed tag cannot be combined with envArgs)"},
		{"image duplicate", func(w *Workspace) { w.Builds[2].Image = w.Builds[1].Image }, "builds[2].image (duplicate"},
		// app-of-apps top-level values are not apps
		{"key env", func(w *Workspace) { w.EnvApps[0].Key = "env" }, "envApps[0].key (env, environment, global, apps are app-of-apps values"},
		{"key global", func(w *Workspace) { w.EnvApps[0].Key = "global" }, "envApps[0].key (env, environment"},
		{"key apps", func(w *Workspace) { w.EnvApps[0].Key = "apps" }, "envApps[0].key (env, environment"},
		// the seed runs before the apps
		{"seed in phase 2", func(w *Workspace) { w.EnvApps[3].Phase = 2 }, "envApps[3].phase (the seed app runs in phase 1"},
		{"seed with repo", func(w *Workspace) { w.EnvApps[3].Repo = "shop-api" }, "envApps[3].seed (the seed app takes no repo, svc or ingress)"},
		{"ingress over image.tag", func(w *Workspace) { w.EnvApps[4].Ingress[0].Values = "image.tag" }, "envApps[4].ingress[0].values (image is the kit's image.tag key)"},
		{"built app in phase 1", func(w *Workspace) { w.EnvApps[4].Phase = 1 }, "envApps[4].phase (an app with repo runs in phase 2)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := testWS(t)
			tc.mod(&w)
			err := w.validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("valid workspace rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// Every problem is listed at once, so one edit of infra.json fixes them all.
func TestWorkspaceValidateListsAllErrors(t *testing.T) {
	w := testWS(t)
	w.HelmRepoURL = "http://x"
	w.Builds[1].Repo = "a/b"
	w.EnvApps[4].Svc = "B"
	err := w.validate()
	for _, want := range []string{"helmRepoURL", "builds[1].repo", "envApps[4].svc"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want %s", err, want)
		}
	}
}
