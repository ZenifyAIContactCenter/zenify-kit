package wt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_Variants(t *testing.T) {
	root := t.TempDir()
	writeWorktreeJSON(t, root, `{"defaultVariant":"js","variants":{
		"js":{"sparseExclude":["android/","ios"]},
		"native":{"submodules":["native/core"],"setup":"pod install"}}}`)
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultVariant != "js" || len(c.Variants) != 2 {
		t.Fatalf("variants not loaded: %+v", c)
	}
	if got := c.Variants["native"]; got.Setup != "pod install" || got.Submodules[0] != "native/core" {
		t.Fatalf("native variant wrong: %+v", got)
	}
}

func TestLoad_VariantsRejectBadConfig(t *testing.T) {
	cases := map[string]string{
		"unknown default":  `{"defaultVariant":"js","variants":{"native":{}}}`,
		"default, no map":  `{"defaultVariant":"js"}`,
		"negated exclude":  `{"variants":{"js":{"sparseExclude":["!android"]}}}`,
		"glob exclude":     `{"variants":{"js":{"sparseExclude":["*.gradle"]}}}`,
		"dotdot exclude":   `{"variants":{"js":{"sparseExclude":["../x"]}}}`,
		"empty exclude":    `{"variants":{"js":{"sparseExclude":["/"]}}}`,
		"bad variant name": `{"variants":{"JS":{}}}`,
		"flag submodule":   `{"variants":{"n":{"submodules":["--recursive"]}}}`,
		"dotdot submodule": `{"variants":{"n":{"submodules":["../x"]}}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeWorktreeJSON(t, root, body)
			if _, err := Load(root); err == nil {
				t.Fatalf("%s must be rejected", body)
			}
		})
	}
}

func TestSparseExcludePattern(t *testing.T) {
	for in, want := range map[string]string{
		"android":    "!/android/",
		"android/":   "!/android/",
		"/native/":   "!/native/",
		"a/b":        "!/a/b/",
		"a//b":       "",
		"./android":  "",
		"android/..": "",
	} {
		if got := sparseExcludePattern(in); got != want {
			t.Errorf("sparseExcludePattern(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveVariant(t *testing.T) {
	c := &Config{DefaultVariant: "js", Variants: map[string]Variant{"js": {Setup: "a"}, "native": {Setup: "b"}}}
	if n, v, err := ResolveVariant(c, ""); err != nil || n != "js" || v.Setup != "a" {
		t.Fatalf("empty request must fall back to default: %q %+v %v", n, v, err)
	}
	if n, v, err := ResolveVariant(c, "native"); err != nil || n != "native" || v.Setup != "b" {
		t.Fatalf("explicit request must win: %q %+v %v", n, v, err)
	}
	if _, _, err := ResolveVariant(c, "web"); err == nil || !strings.Contains(err.Error(), "js, native") {
		t.Fatalf("unknown variant must list declared ones, got %v", err)
	}
	if n, _, err := ResolveVariant(&Config{}, ""); err != nil || n != "" {
		t.Fatalf("no variants and no request = plain checkout, got %q %v", n, err)
	}
	if _, _, err := ResolveVariant(&Config{}, "js"); err == nil || !strings.Contains(err.Error(), "declares no variants") {
		t.Fatalf("request on a repo without variants must fail, got %v", err)
	}
}

// variantRepo builds a repo on main with app code plus native dirs, and an
// optional local submodule at native/core.
func variantRepo(t *testing.T, cfg string, withSubmodule bool) string {
	t.Helper()
	root, _ := pushRepo(t, true)
	writeWorktreeJSON(t, root, cfg)
	for _, f := range []string{"src/app.js", "android/build.gradle", "ios/Podfile", "native/README"} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if withSubmodule {
		// Local file:// submodules are blocked by default since git 2.38.1;
		// allow them for this test process only (real git, both test and RunNew).
		t.Setenv("GIT_CONFIG_COUNT", "1")
		t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
		t.Setenv("GIT_CONFIG_VALUE_0", "always")
		sub := t.TempDir()
		gitT(t, sub, "init", "-q")
		if err := os.WriteFile(filepath.Join(sub, "core.c"), []byte("int x;"), 0o600); err != nil {
			t.Fatal(err)
		}
		gitT(t, sub, "add", "-A")
		gitT(t, sub, "commit", "-q", "-m", "core")
		gitT(t, root, "submodule", "add", "-q", sub, "native/core")
	}
	gitT(t, root, "add", "-A")
	gitT(t, root, "commit", "-q", "-m", "app")
	gitT(t, root, "push", "-q", "origin", "main")
	gitT(t, root, "fetch", "-q", "origin")
	return root
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestRunNew_DefaultVariantHidesNativeDirs(t *testing.T) {
	root := variantRepo(t, `{"abbrev":"m","user":"namph","portRange":[3200,3249],"deps":"none",
		"defaultVariant":"js","variants":{"js":{"sparseExclude":["android/","ios/","native/"]},"native":{}}}`, false)
	o, errb := realOpts(root)
	if err := RunNew(o); err != nil {
		t.Fatalf("RunNew: %v\n%s", err, errb)
	}
	wtPath := filepath.Join(root, ".worktrees", "my-task")
	if !exists(filepath.Join(wtPath, "src", "app.js")) {
		t.Fatal("JS code must be checked out")
	}
	for _, d := range []string{"android", "ios", "native"} {
		if exists(filepath.Join(wtPath, d)) {
			t.Fatalf("%s/ must be hidden in the js worktree", d)
		}
		if !exists(filepath.Join(root, d)) {
			t.Fatalf("%s/ must stay in the main checkout", d)
		}
	}
	if got := strings.TrimSpace(gitT(t, wtPath, "config", "--worktree", "wt.variant")); got != "js" {
		t.Fatalf("wt.variant = %q, want js", got)
	}
	if st := strings.TrimSpace(gitT(t, wtPath, "status", "--porcelain")); st != "" {
		t.Fatalf("hidden dirs must not show as deleted, status:\n%s", st)
	}
	if !strings.Contains(errb.String(), "variant=js") {
		t.Fatalf("tail line must name the variant, got %q", errb.String())
	}
}

func TestRunNew_NativeVariantInitsSubmoduleAndRunsSetup(t *testing.T) {
	root := variantRepo(t, `{"abbrev":"m","user":"namph","portRange":[3200,3249],"deps":"none",
		"defaultVariant":"js","variants":{"js":{"sparseExclude":["android/","ios/","native/"]},
		"native":{"submodules":["native/core"],"setup":"touch setup-ran"}}}`, true)
	o, errb := realOpts(root)
	o.Variant = "native"
	if err := RunNew(o); err != nil {
		t.Fatalf("RunNew: %v\n%s", err, errb)
	}
	wtPath := filepath.Join(root, ".worktrees", "my-task")
	for _, f := range []string{"android/build.gradle", "ios/Podfile", "native/core/core.c", "setup-ran"} {
		if !exists(filepath.Join(wtPath, f)) {
			t.Fatalf("native worktree must have %s", f)
		}
	}
}

func TestRunNew_UnknownVariantFailsBeforeTouchingDisk(t *testing.T) {
	root := variantRepo(t, `{"abbrev":"m","user":"namph","portRange":[3200,3249],"deps":"none",
		"variants":{"js":{}}}`, false)
	o, _ := realOpts(root)
	o.Variant = "web"
	if err := RunNew(o); err == nil || !strings.Contains(err.Error(), "has: js") {
		t.Fatalf("unknown variant must fail with the declared list, got %v", err)
	}
	if exists(filepath.Join(root, ".worktrees", "my-task")) {
		t.Fatal("no worktree may be created for an unknown variant")
	}
}

func TestRunNew_SetupFailureAbortsCleanly(t *testing.T) {
	root := variantRepo(t, `{"abbrev":"m","user":"namph","portRange":[3200,3249],"deps":"none",
		"defaultVariant":"native","variants":{"native":{"setup":"exit 3"}}}`, false)
	o, _ := realOpts(root)
	if err := RunNew(o); err == nil || !strings.Contains(err.Error(), `variant "native" setup failed`) {
		t.Fatalf("setup failure must abort, got %v", err)
	}
	if exists(filepath.Join(root, ".worktrees", "my-task")) {
		t.Fatal("aborted run must leave no worktree")
	}
	if out := gitT(t, root, "branch", "--list", "namph/feat/my-task"); strings.TrimSpace(out) != "" {
		t.Fatalf("aborted run must delete the branch, got %q", out)
	}
}
