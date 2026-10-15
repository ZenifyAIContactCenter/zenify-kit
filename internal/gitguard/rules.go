package gitguard

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// ProdHost is one production host and the _PRD key it came from.
type ProdHost struct{ Key, Host string } // Host lower-cased hostname

// Options carries what the rules need from the workspace.
type Options struct {
	ProdHosts []ProdHost
	EnvFile   string // absolute path of the workspace .zenify/.env; "" = unknown
}

func denyHint(sub, why, hint string) Decision {
	return Decision{Deny: true, Message: "🚫 [git-guard] BLOCKED — " + why + " (" + sub + "). " + hint}
}

var (
	denyForcePush = denyHint("push", "force push", "Chạy tay bằng ! nếu thật sự cần.")                                      //znf:allow-lang
	denyPRMerge   = denyHint("gh", "merge PR", "Merge là việc của người: chạy tay bằng ! nếu thật sự cần.")                 //znf:allow-lang
	denyWtForce   = denyHint("wt", "wt rm --force", "Worktree có thể chưa merge — chạy tay bằng ! nếu thật sự cần.")        //znf:allow-lang
	denyEnvFile   = denyHint("env-file", "đọc .zenify/.env", "Dùng zenify db-read hoặc zenify env list.")                   //znf:allow-lang
	denyParse     = denyHint("parse", "lệnh không phân tích được có từ khoá nguy hiểm", "Viết lại lệnh, không lồng chuỗi.") //znf:allow-lang
)

func denyProdHost(key string) Decision {
	return denyHint("prod-host", "host của "+key, "Production chỉ đọc qua zenify db-read.") //znf:allow-lang
}

// checkRules applies R2–R7 to a parsed command; Deny=false when none fires.
func checkRules(p Parsed, raw, base string, getenv func(string) string, opts Options) Decision {
	envFile := canonical(opts.EnvFile)
	for _, c := range p.Cmds {
		switch {
		case isForcePush(c.Argv):
			return denyForcePush
		case isPRMerge(c.Argv):
			return denyPRMerge
		case isWtForceRm(c.Argv):
			return denyWtForce
		}
		if exempt(c) {
			continue
		}
		if c.PRDExpansion || isPrintenvPRD(c.Argv) ||
			readsEnvFile(slices.Concat(c.Argv, c.RedirArgv), base, envFile, getenv) {
			return denyEnvFile
		}
		if key := matchHost(strings.ToLower(strings.Join(slices.Concat(c.Src, c.RedirSrc), " ")), opts.ProdHosts); key != "" {
			return denyProdHost(key)
		}
	}
	if p.PRDExpansion { // _PRD expansion outside any Cmd (assignment, redirect, declaration)
		return denyEnvFile
	}
	if p.Unparseable && dangerousRaw(raw, opts.ProdHosts) {
		return denyParse
	}
	return Decision{}
}

// isForcePush: R2 — git push with a force flag or a '+' refspec.
func isForcePush(argv []string) bool {
	if !isGitBinary(argv[0]) {
		return false
	}
	gc, ok := parseGitCall(argv)
	if !ok || gc.Sub != "push" {
		return false
	}
	for _, a := range gc.Args {
		switch {
		case a == "--force", a == "--force-with-lease", a == "--force-if-includes",
			strings.HasPrefix(a, "--force-with-lease="),
			strings.HasPrefix(a, "+"):
			return true
		case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "f"):
			return true
		}
	}
	return false
}

// isPRMerge: R3 — gh pr merge, or gh api PUT …/pulls/<n>/merge.
func isPRMerge(argv []string) bool {
	if basename(argv[0]) != "gh" || len(argv) < 2 {
		return false
	}
	if argv[1] == "pr" {
		return len(argv) > 2 && argv[2] == "merge"
	}
	if argv[1] != "api" {
		return false
	}
	var mergePath, put bool
	for i, a := range argv[2:] {
		next := ""
		if i+3 < len(argv) {
			next = argv[i+3]
		}
		switch {
		case strings.Contains(a, "/pulls/") && strings.HasSuffix(a, "/merge"):
			mergePath = true
		case (a == "-X" || a == "--method") && strings.EqualFold(next, "PUT"),
			strings.EqualFold(a, "-XPUT"), strings.EqualFold(a, "--method=PUT"):
			put = true
		}
	}
	return mergePath && put
}

// isWtForceRm: R4 — `wt rm` / `zenify wt rm` / `git worktree remove` with
// --force or -f.
func isWtForceRm(argv []string) bool {
	var rest []string
	switch name := basename(argv[0]); {
	case isGitBinary(argv[0]):
		gc, ok := parseGitCall(argv)
		if !ok || gc.Sub != "worktree" || len(gc.Args) == 0 || gc.Args[0] != "remove" {
			return false
		}
		rest = gc.Args[1:]
	case name == "wt":
		if len(argv) < 2 || argv[1] != "rm" {
			return false
		}
		rest = argv[2:]
	case name == "zenify":
		if len(argv) < 3 || argv[1] != "wt" || argv[2] != "rm" {
			return false
		}
		rest = argv[3:]
	default:
		return false
	}
	return slices.Contains(rest, "--force") || slices.Contains(rest, "-f")
}

// exempt: FR-5.2 commands that R5 and R6 skip.
func exempt(c Cmd) bool {
	a := c.Argv
	switch basename(a[0]) {
	case "zenify":
		return len(a) > 1 && (a[1] == "db-read" ||
			(a[1] == "env" && len(a) > 2 && slices.Contains([]string{"list", "set", "edit"}, a[2])))
	case "chmod", "ls", "stat", "test", "[":
		return true
	}
	return false
}

func isPrintenvPRD(argv []string) bool {
	if basename(argv[0]) != "printenv" {
		return false
	}
	for _, a := range argv[1:] {
		if strings.HasSuffix(a, "_PRD") {
			return true
		}
	}
	return false
}

// readsEnvFile: some word names a file called .env that resolves, from base,
// to the workspace .zenify/.env.
func readsEnvFile(argv []string, base, envFile string, getenv func(string) string) bool {
	if envFile == "" {
		return false
	}
	for _, a := range argv {
		if a == "" || filepath.Base(a) != ".env" {
			continue
		}
		if canonical(resolveDir(base, a, getenv)) == envFile {
			return true
		}
	}
	return false
}

// canonical cleans path and resolves symlinks when it can.
func canonical(path string) string {
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return path
}

func isHostByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '.' || b == '-'
}

// matchHost returns the key of the first host found in s (lower-cased) at a
// host boundary, "" when none.
func matchHost(s string, hosts []ProdHost) string {
	for _, h := range hosts {
		if h.Host == "" {
			continue
		}
		for i := 0; ; {
			j := strings.Index(s[i:], h.Host)
			if j < 0 {
				break
			}
			start, end := i+j, i+j+len(h.Host)
			if (start == 0 || !isHostByte(s[start-1])) && (end == len(s) || !isHostByte(s[end])) {
				return h.Key
			}
			i = start + 1
		}
	}
	return ""
}

var (
	reRawPushF  = regexp.MustCompile(`push.* -f( |$)`)
	reRawPRDVar = regexp.MustCompile(`\$\{?[a-z0-9_]*_prd\b`)
)

// dangerousRaw: R7 keywords in the raw command (quotes dropped, lower-cased).
func dangerousRaw(raw string, hosts []ProdHost) bool {
	s := strings.ToLower(quoteStripper.Replace(raw))
	for _, k := range []string{"pr merge", "/merge", "--force", ".zenify/.env"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return reRawPushF.MatchString(s) || reRawPRDVar.MatchString(s) || matchHost(s, hosts) != ""
}
