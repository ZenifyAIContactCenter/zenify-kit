package update

import "strings"

// Method is how this binary got onto the machine — it decides which upgrade
// command is the right one to print or run.
type Method int

const (
	Unknown Method = iota
	Brew           // brew install --cask zenify (Caskroom or Cellar in the path)
	Scoop          // scoop install zenify (a "scoop" segment in the path)
	Script         // scripts/install.sh → ~/.local/bin, install.ps1 → %LOCALAPPDATA%\Programs\zenify
)

// InstallDoc is where the README explains every install path.
const InstallDoc = "https://github.com/ZenifyAIContactCenter/zenify-kit#install"

const (
	installSh  = "curl -fsSL https://raw.githubusercontent.com/ZenifyAIContactCenter/zenify-kit/main/scripts/install.sh | sh"
	installPs1 = "irm https://raw.githubusercontent.com/ZenifyAIContactCenter/zenify-kit/main/scripts/install.ps1 | iex"
)

func (m Method) String() string {
	switch m {
	case Brew:
		return "brew"
	case Scoop:
		return "scoop"
	case Script:
		return "install script"
	default:
		return "unknown"
	}
}

// Detect classifies the resolved executable path. The caller passes exe after
// filepath.EvalSymlinks (brew's /opt/homebrew/bin/zenify is a symlink into
// Caskroom), runtime.GOOS, the user home and, on Windows, %LOCALAPPDATA%.
// Both path separators are accepted so Windows paths can be classified in
// tests on any OS.
func Detect(exe, goos, home, localAppData string) Method {
	if exe == "" {
		return Unknown
	}
	home = strings.TrimRight(home, `/\`)
	localAppData = strings.TrimRight(localAppData, `/\`)
	for _, seg := range strings.FieldsFunc(exe, isSep) {
		switch {
		case seg == "Caskroom" || seg == "Cellar":
			return Brew
		case goos == "windows" && strings.EqualFold(seg, "scoop"):
			return Scoop
		}
	}
	if goos == "windows" {
		if localAppData != "" && hasPrefixFold(exe, localAppData+`\Programs\zenify\`) {
			return Script
		}
		return Unknown
	}
	if home != "" && strings.HasPrefix(exe, home+"/.local/bin/") {
		return Script
	}
	return Unknown
}

// Command returns the executable + args to run for m, and a one-line display
// form for messages. Unknown returns an empty name and points at the README.
func Command(m Method, goos string) (name string, args []string, display string) {
	switch m {
	case Brew:
		return "brew", []string{"upgrade", "--cask", "zenify"}, "brew upgrade --cask zenify"
	case Scoop:
		return "scoop", []string{"update", "zenify"}, "scoop update zenify"
	case Script:
		if goos == "windows" {
			return "powershell", []string{"-NoProfile", "-Command", installPs1}, installPs1
		}
		return "sh", []string{"-c", installSh}, installSh
	default:
		return "", nil, "see " + InstallDoc
	}
}

func isSep(r rune) bool { return r == '/' || r == '\\' }

// hasPrefixFold is a case-insensitive HasPrefix — Windows paths are
// case-insensitive and %LOCALAPPDATA% casing varies between shells.
func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}
