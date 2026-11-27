package infra

import (
	"strings"
	"testing"
)

// renderFirewall renders the nft file exactly as InstallK3s writes it.
func renderFirewall(t *testing.T) string {
	t.Helper()
	for _, f := range k3sHostFiles() {
		if f.path == nftPath {
			b, err := f.render()
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	t.Fatal("no host file for " + nftPath)
	return ""
}

// Business rule (FR-2.8): the k3s API server (6443) and kubelet (10250) must
// not be reachable from the LAN. Only loopback and the pod (10.42.0.0/16) and
// service (10.43.0.0/16) CIDRs may reach them, the accepts come before the
// drop, and the table filters nothing else (no other port is opened or closed).
func TestFirewallAPIServerAndKubeletUnreachableFromLAN(t *testing.T) {
	nft := renderFirewall(t)
	start := strings.Index(nft, "chain input {")
	if start < 0 {
		t.Fatalf("no input chain:\n%s", nft)
	}
	body := nft[start+len("chain input {"):]
	body = body[:strings.Index(body, "}\n")] // end of the chain
	var rules []string
	for _, l := range strings.Split(body, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			rules = append(rules, l)
		}
	}
	want := []string{
		"type filter hook input priority filter; policy accept;",
		`iifname "lo" accept`,
		"ip saddr { 10.42.0.0/16, 10.43.0.0/16 } accept",
		"tcp dport { 6443, 10250 } drop",
	}
	if strings.Join(rules, "\n") != strings.Join(want, "\n") {
		t.Fatalf("input chain rules =\n%s\nwant\n%s", strings.Join(rules, "\n"), strings.Join(want, "\n"))
	}
	if strings.Count(nft, "chain ") != 1 {
		t.Errorf("only the input chain may exist:\n%s", nft)
	}
	if !strings.Contains(nft, "table inet "+nftTable+" {") || !strings.Contains(nft, "delete table inet "+nftTable) {
		t.Errorf("rules must live in the dedicated table %s, replaced atomically:\n%s", nftTable, nft)
	}
}
