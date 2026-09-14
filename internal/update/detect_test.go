package update

import (
	"strings"
	"testing"
)

// SC-6
func TestDetect(t *testing.T) {
	cases := []struct {
		name, exe, goos, home, lad string
		want                       Method
	}{
		{"brew mac", "/opt/homebrew/Caskroom/zenify/0.17.3/zenify", "darwin", "/Users/a", "", Brew},
		{"brew intel mac", "/usr/local/Caskroom/zenify/0.17.3/zenify", "darwin", "/Users/a", "", Brew},
		{"brew linux", "/home/linuxbrew/.linuxbrew/Caskroom/zenify/0.17.3/zenify", "linux", "/home/a", "", Brew},
		{"brew cellar", "/opt/homebrew/Cellar/zenify/0.17.3/bin/zenify", "darwin", "/Users/a", "", Brew},
		{"scoop", `C:\Users\a\scoop\apps\zenify\current\zenify.exe`, "windows", `C:\Users\a`, `C:\Users\a\AppData\Local`, Scoop},
		{"scoop upper", `C:\Users\a\Scoop\shims\zenify.exe`, "windows", `C:\Users\a`, `C:\Users\a\AppData\Local`, Scoop},
		{"script unix", "/Users/a/.local/bin/zenify", "darwin", "/Users/a", "", Script},
		{"script linux", "/home/a/.local/bin/zenify", "linux", "/home/a", "", Script},
		{"script windows", `C:\Users\a\AppData\Local\Programs\zenify\zenify.exe`, "windows", `C:\Users\a`, `C:\Users\a\AppData\Local`, Script},
		{"script windows case", `c:\users\a\appdata\local\programs\zenify\zenify.exe`, "windows", `C:\Users\a`, `C:\Users\a\AppData\Local`, Script},
		{"unknown usr local", "/usr/local/bin/zenify", "darwin", "/Users/a", "", Unknown},
		{"unknown go build", "/Users/a/WorkingSpace/zenify-kit/zenify", "darwin", "/Users/a", "", Unknown},
		{"unknown empty home", "/.local/bin/zenify", "linux", "", "", Unknown},
		{"unknown empty lad", `C:\Programs\zenify\zenify.exe`, "windows", `C:\Users\a`, "", Unknown},
		{"unknown empty exe", "", "darwin", "/Users/a", "", Unknown},
		{"script unix trailing home sep", "/Users/a/.local/bin/zenify", "darwin", "/Users/a/", "", Script},
		{"script windows trailing lad sep", `C:\Users\a\AppData\Local\Programs\zenify\zenify.exe`, "windows", `C:\Users\a`, `C:\Users\a\AppData\Local\`, Script},
		{"scoop segment non-windows is unknown", "/Users/a/scoop/zenify", "darwin", "/Users/a", "", Unknown},
	}
	for _, c := range cases {
		if got := Detect(c.exe, c.goos, c.home, c.lad); got != c.want {
			t.Errorf("%s: Detect = %v, want %v", c.name, got, c.want)
		}
	}
}

// SC-7
func TestCommand(t *testing.T) {
	name, args, display := Command(Brew, "darwin")
	if name != "brew" || strings.Join(args, " ") != "upgrade --cask zenify" || display != "brew upgrade --cask zenify" {
		t.Fatalf("brew: %q %q %q", name, args, display)
	}
	name, args, display = Command(Scoop, "windows")
	if name != "scoop" || strings.Join(args, " ") != "update zenify" || display != "scoop update zenify" {
		t.Fatalf("scoop: %q %q %q", name, args, display)
	}
	name, args, display = Command(Script, "darwin")
	if name != "sh" || len(args) != 2 || args[0] != "-c" || !strings.Contains(args[1], "scripts/install.sh | sh") || display != args[1] {
		t.Fatalf("script unix: %q %q %q", name, args, display)
	}
	name, args, display = Command(Script, "windows")
	if name != "powershell" || len(args) != 3 || args[0] != "-NoProfile" || args[1] != "-Command" || !strings.Contains(args[2], "scripts/install.ps1 | iex") || display != args[2] {
		t.Fatalf("script windows: %q %q %q", name, args, display)
	}
	name, args, display = Command(Unknown, "darwin")
	if name != "" || args != nil || !strings.Contains(display, "#install") {
		t.Fatalf("unknown: %q %q %q", name, args, display)
	}
}

func TestMethodString(t *testing.T) {
	for m, want := range map[Method]string{Unknown: "unknown", Brew: "brew", Scoop: "scoop", Script: "install script"} {
		if m.String() != want {
			t.Errorf("%d.String() = %q, want %q", m, m.String(), want)
		}
	}
}
