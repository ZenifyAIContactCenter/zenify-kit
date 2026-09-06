package cli

import "testing"

func TestDecideMode(t *testing.T) {
	cases := []struct {
		name                                             string
		isTTY, apply, dryRunSet, dryRun, jsonOut, nonInt bool
		want                                             upMode
	}{
		{"tty no flags -> wizard", true, false, false, false, false, false, modeWizard},
		{"tty --apply -> apply", true, true, false, false, false, false, modeApply},
		{"tty --dry-run -> dryrun", true, false, true, true, false, false, modeDryRun},
		{"nonTTY no flags -> dryrun (preserve print-plan, no hang)", false, false, false, false, false, false, modeDryRun},
		{"nonTTY --apply -> apply", false, true, false, false, false, false, modeApply},
		{"json no apply -> dryrun", true, false, false, false, true, false, modeDryRun},
		{"json --apply -> apply", true, true, false, false, true, false, modeApply},
		{"nonInteractive no apply -> dryrun", true, false, false, false, false, true, modeDryRun},
		{"apply beats dry-run default in headless", false, true, false, false, true, false, modeApply},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := decideMode(c.isTTY, c.apply, c.dryRunSet, c.dryRun, c.jsonOut, c.nonInt)
			if got != c.want {
				t.Fatalf("decideMode = %v, want %v", got, c.want)
			}
		})
	}
}
