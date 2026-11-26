package infra

import (
	"context"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// Business rule: the k3s admin kubeconfig is cluster-admin (it reaches the
// repo token Secret and the CA key); it must not be world-readable, only
// readable by the devbox user's group so kubectl still works without sudo.
func TestK3sConfigKubeconfigNotWorldReadable(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		t.Fatal(err)
	}
	cfg := desiredHostFiles(t)[k3sConfigPath]
	if !strings.Contains(cfg, `write-kubeconfig-mode: "0640"`) || !strings.Contains(cfg, `write-kubeconfig-group: "`+g.Name+`"`) {
		t.Fatalf("k3s config:\n%s", cfg)
	}
	if strings.Contains(cfg, "0644") {
		t.Fatalf("kubeconfig still world-readable:\n%s", cfg)
	}
}

// Business rule: sudo exits 1 on its own failures (password needed, policy);
// that must not read as "file absent", or down --purge would skip the k3s
// uninstall and the firewall removal and still report success.
func TestReadRootFileSudoFailureIsAnError(t *testing.T) {
	for code, wantErr := range map[int]bool{1: true, rootFileAbsent: false} {
		r := &fakeRunner{respond: func(c call) ([]byte, error) {
			if isProbe(c) {
				return nil, exitErr(code)
			}
			return nil, nil
		}}
		_, exists, err := readRootFile(context.Background(), r, "/etc/x")
		if (err != nil) != wantErr || exists {
			t.Errorf("exit %d: exists=%v err=%v, want error=%v", code, exists, err, wantErr)
		}
	}
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if isProbe(c) {
			return nil, exitErr(1)
		}
		return nil, nil
	}}
	if err := UninstallK3s(context.Background(), r); err == nil || r.ran("sudo "+k3sUninstall) {
		t.Fatalf("uninstall err=%v calls=%v: a sudo failure must stop, not skip", err, r.calls)
	}
}

// Business rule: k3s must not come up unfiltered: the firewall unit is
// required by k3s.service, and restarting it (which restarts k3s) waits for
// the node before up goes on.
func TestFirewallRequiredByK3s(t *testing.T) {
	unit := desiredHostFiles(t)[unitPath]
	if !strings.Contains(unit, "RequiredBy=k3s.service") {
		t.Fatalf("firewall unit:\n%s", unit)
	}
	files := desiredHostFiles(t)
	delete(files, nftPath)
	r := &fakeRunner{respond: hostFiles(files, K3sVersion)}
	if _, err := InstallK3s(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	restart, nodes := -1, -1
	for i, c := range r.calls {
		if c.line() == "sudo systemctl restart "+firewallUnit {
			restart = i
		}
		if c.name == "kubectl" && strings.Contains(c.line(), "get nodes") && restart >= 0 && nodes < 0 {
			nodes = i
		}
	}
	if restart < 0 || nodes < restart {
		t.Fatalf("no node wait after the firewall restart: %v", r.calls)
	}
}

func preflightMissing(t *testing.T, cmd, fix string) {
	t.Helper()
	p := okProbe()
	p.cmds[cmd] = false
	got := Preflight(p, DefaultRequirements, testPin)
	if len(got) != 1 || got[0].Check != "tools" || !strings.Contains(got[0].Fix, fix) {
		t.Fatalf("missing %s: %+v", cmd, got)
	}
}

// Business rule: the firewall needs nft and split DNS needs resolvectl; a host
// without them must fail preflight instead of failing midway through up.
func TestPreflightRequiresNftAndResolvectl(t *testing.T) {
	preflightMissing(t, "nft", "nftables")
	preflightMissing(t, "resolvectl", "systemd-resolved")
}

// Business rule: the kit only ever removes its own temp worktree; a global
// `git worktree prune` would drop admin entries of the user's worktrees.
func TestBaseSourceNoGlobalPrune(t *testing.T) {
	buildEnv(t, http.StatusOK)
	ws := fakeWorkspace(t)
	home, _ := os.UserHomeDir()
	dir := filepath.Join(infraDir(home), "src", "zenify-data-seed-0123456")
	repo := filepath.Join(ws, "repos", "zenify-data-seed")
	run := func(listed, exists bool) *fakeRunner {
		t.Helper()
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		if exists {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		r := &fakeRunner{respond: func(c call) ([]byte, error) {
			if strings.Contains(c.line(), "worktree list --porcelain") {
				out := "worktree " + repo + "\nHEAD abc\n\nworktree /home/u/user-wt\nHEAD def\n"
				if listed {
					out += "\nworktree " + dir + "\nHEAD 0123\ndetached\n"
				}
				return []byte(out), nil
			}
			return gitFake(false)(c)
		}}
		if _, _, cleanup, err := BaseSource(context.Background(), r, ws, "zenify-data-seed", ""); err != nil {
			t.Fatal(err)
		} else if cleanup == nil {
			t.Fatal("no cleanup")
		}
		for _, c := range r.calls {
			if strings.Contains(c.line(), "worktree prune") {
				t.Fatalf("global prune ran: %s", c.line())
			}
			if strings.Contains(c.line(), "worktree remove") && !strings.Contains(c.line(), dir) {
				t.Fatalf("removed a worktree that is not the kit's: %s", c.line())
			}
		}
		return r
	}
	if r := run(true, true); !r.ran("git -C "+repo+" worktree remove --force "+dir) || !r.ran("git -C "+repo+" worktree add --detach "+dir) {
		t.Fatalf("listed leftover not replaced: %v", r.calls)
	}
	if r := run(true, false); !r.ran("git -C " + repo + " worktree add --force --detach " + dir) {
		t.Fatalf("registered-but-deleted entry not reused: %v", r.calls)
	}
	if r := run(false, true); r.ran("git -C "+repo+" worktree remove") || !r.ran("git -C "+repo+" worktree add --detach "+dir) {
		t.Fatalf("unlisted leftover: %v", r.calls)
	}
}
