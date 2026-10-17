package gitguard

import (
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Options carries what the rules need from the workspace.
type Options struct {
	EnvFile string // absolute path of the workspace .zenify/.env; "" = unknown
}

func denyHint(sub, why, hint string) Decision {
	return Decision{Deny: true, Message: "🚫 [git-guard] BLOCKED — " + why + " (" + sub + "). " + hint}
}

// stopHint ends every deny that only a human may lift.
const stopHint = " Dừng và báo người dùng; không thử biến thể." //znf:allow-lang

var (
	denyForcePush  = denyHint("push", "force push", "Chạy tay bằng ! nếu thật sự cần."+stopHint)                                                                                                                                           //znf:allow-lang
	denyPRMerge    = denyHint("gh", "merge PR", "Merge là việc của người: chạy tay bằng ! nếu thật sự cần."+stopHint)                                                                                                                      //znf:allow-lang
	denyStack      = denyHint("gh", "merge/gỡ/ghép stack", "Merge hay gỡ stack là việc của người: chạy tay bằng ! nếu thật sự cần."+stopHint)                                                                                              //znf:allow-lang
	denyStackAlias = denyHint("gh", "gh stack alias", "Alias tạo lệnh tắt (gs) mà guard không nhận ra: chạy tay bằng ! nếu thật sự cần."+stopHint)                                                                                         //znf:allow-lang
	denyWtForce    = denyHint("wt", "wt rm --force", "Worktree có thể chưa merge — chạy tay bằng ! nếu thật sự cần."+stopHint)                                                                                                             //znf:allow-lang
	denyEnvFile    = denyHint("env-file", "đọc .zenify/.env", "Không đọc .zenify/.env. Cần DB: đọc bằng zenify db-read / redis-read; chạy script bằng zenify data run --stg|--prd -- <lệnh>.")                                             //znf:allow-lang
	denyParse      = denyHint("parse", "lệnh không phân tích được có từ khoá nguy hiểm", "Viết lại thành lệnh đơn, không lồng chuỗi; script DB chạy thẳng qua zenify data run. Nếu lệnh thật sự là merge, force push hay stack:"+stopHint) //znf:allow-lang
	denyManifest   = denyHint("zenify", "--manifest cho db-read/redis-read", "Bỏ --manifest: lệnh tự dùng manifest nhúng sẵn. Manifest khác đổi broker nhận GitHub token, việc của người."+stopHint)                                       //znf:allow-lang
)

// checkRules applies R2–R5, R7 and R8 to a parsed command; Deny=false when none fires.
func checkRules(p Parsed, raw, base string, getenv func(string) string, opts Options) Decision {
	envFile := canonical(opts.EnvFile)
	for _, c := range p.Cmds {
		switch {
		case isForcePush(c.Argv):
			return denyForcePush
		case isPRMerge(c.Argv):
			return denyPRMerge
		case isStackAlias(c.Argv):
			return denyStackAlias
		case isStackForbidden(c.Argv):
			return denyStack
		case isWtForceRm(c.Argv):
			return denyWtForce
		case isBrokerManifestFlag(c.Argv):
			return denyManifest
		}
		if exempt(c) {
			continue
		}
		if readsEnvFile(slices.Concat(c.Argv, c.RedirArgv), base, envFile, getenv) {
			return denyEnvFile
		}
	}
	if p.Unparseable && dangerousRaw(raw) {
		return denyParse
	}
	return Decision{}
}

// isForcePush: R2 — git push with a force flag or a '+' refspec.
func isForcePush(argv []string) bool {
	if !isGitBinary(argv[0]) {
		return false
	}
	for _, gc := range gitCallVariants(argv) {
		if gc.Sub != "push" {
			continue
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

var reStackUnstack = regexp.MustCompile(`(^|/)repos/[^/]+/[^/]+/stacks/\d+/unstack/?(\?.*)?$`)

// stackCall returns the gh-stack subcommand and its args for `gh stack …` or
// a direct `gh-stack …` call; ok is false for anything else.
func stackCall(argv []string) (sub string, args []string, ok bool) {
	rest := argv[1:]
	switch basename(argv[0]) {
	case "gh":
		if len(rest) < 1 || rest[0] != "stack" {
			return "", nil, false
		}
		rest = rest[1:]
	case "gh-stack":
	default:
		return "", nil, false
	}
	if len(rest) == 0 {
		return "", nil, true
	}
	return rest[0], rest[1:], true
}

// isStackAlias: R8 — `gh stack alias` installs a wrapper (default `gs`) that
// stackCall cannot recognise, so `gs merge` would pass the guard.
func isStackAlias(argv []string) bool {
	sub, _, ok := stackCall(argv)
	return ok && sub == "alias"
}

// isStackForbidden: R8 — merging, unstacking (server side) or linking a stack.
func isStackForbidden(argv []string) bool {
	if basename(argv[0]) == "gh" && len(argv) > 1 && argv[1] == "api" {
		for _, a := range argv[2:] {
			if strings.Contains(a, "merge-async") || reStackUnstack.MatchString(a) {
				return true
			}
		}
		return false
	}
	sub, args, ok := stackCall(argv)
	if !ok {
		return false
	}
	switch sub {
	case "merge", "link":
		return true
	case "unstack", "delete":
		return !slices.Contains(args, "--local")
	}
	return false
}

// stackBranchArgs returns the positional branch names of `gh stack init|add`,
// dropping the -b/--base and -m/--message flags together with their values.
func stackBranchArgs(argv []string) []string {
	sub, args, ok := stackCall(argv)
	if !ok || (sub != "init" && sub != "add") {
		return nil
	}
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-b", a == "--base", a == "-m", a == "--message":
			i++
		case strings.HasPrefix(a, "--base="), strings.HasPrefix(a, "--message="):
		case strings.HasPrefix(a, "-"):
		default:
			out = append(out, a)
		}
	}
	return out
}

// isWtForceRm: R4 — `wt rm` / `zenify wt rm` / `git worktree remove` with
// --force or -f.
func isWtForceRm(argv []string) bool {
	var rest []string
	switch name := basename(argv[0]); {
	case isGitBinary(argv[0]):
		for _, gc := range gitCallVariants(argv) {
			if gc.Sub == "worktree" && len(gc.Args) > 0 && gc.Args[0] == "remove" {
				rest = append(rest, gc.Args[1:]...)
			}
		}
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

// exempt: FR-5.2 commands that R5 skips. `zenify data run … --` is not
// here: Parse strips it as a wrapper, so its child is checked.
func exempt(c Cmd) bool {
	a := c.Argv
	switch basename(a[0]) {
	case "zenify":
		sub := zenifySub(a)
		return len(sub) > 0 && (sub[0] == "db-read" || sub[0] == "redis-read" ||
			(sub[0] == "env" && len(sub) > 1 && slices.Contains([]string{"list", "set", "edit"}, sub[1])))
	case "chmod", "ls", "stat", "test", "[":
		return true
	}
	return false
}

// isBrokerManifestFlag: `zenify db-read|redis-read --manifest …`. Those two run
// without a prompt and outside the sandbox, and --manifest picks the broker URL
// that receives the developer's GitHub token.
func isBrokerManifestFlag(argv []string) bool {
	if basename(argv[0]) != "zenify" {
		return false
	}
	// Any word, not just the first: a flag value before the subcommand
	// (`zenify --manifest m db-read`) shifts it.
	if sub := zenifySub(argv); !slices.Contains(sub, "db-read") && !slices.Contains(sub, "redis-read") {
		return false
	}
	return slices.ContainsFunc(argv, func(t string) bool {
		return t == "--manifest" || strings.HasPrefix(t, "--manifest=")
	})
}

// dirReaders read or copy every file of a directory named on their command line.
var dirReaders = []string{"grep", "rg", "ag", "find", "tar", "zip", "cp", "rsync",
	"cat", "head", "tail", "less", "more", "strings", "xxd", "od"}

// readsEnvFile: some word resolves, from base, to the workspace .zenify/.env
// or to a .env name or a glob inside .zenify (outside .zenify/data-runs, the
// `zenify data run` records); or argv[0] is a dirReader and a word resolves to
// the .zenify directory itself.
func readsEnvFile(argv []string, base, envFile string, getenv func(string) string) bool {
	if envFile == "" {
		return false
	}
	zdir := filepath.Dir(envFile)
	runs := filepath.Join(zdir, "data-runs")
	reader := slices.Contains(dirReaders, basename(argv[0]))
	linker := basename(argv[0]) == "ln"
	under := func(d, dir string) bool { return d == dir || strings.HasPrefix(d, dir+string(filepath.Separator)) }
	for _, w := range argv {
		for _, a := range expandBraces(w) {
			if a == "" {
				continue
			}
			p := resolveDir(base, a, getenv)
			if cp := canonical(p); cp == envFile || reader && cp == zdir {
				return true
			}
			name, d := filepath.Base(p), canonical(filepath.Dir(p))
			if linker && under(d, zdir) { // a link planted in .zenify can point anywhere
				return true
			}
			// The data-runs records are readable; a `..` or a glob in the directory
			// part (filepath.Dir cleans `*/..` away) never earns that, and a glob is
			// expanded here so a link it reaches cannot lead back to .env.
			if !strings.Contains(a, "..") && !strings.ContainsAny(a[:max(strings.LastIndex(a, "/"), 0)], "*?[") && under(d, runs) {
				if m, _ := filepath.Glob(p); slices.ContainsFunc(m, func(f string) bool { return canonical(f) == envFile }) {
					return true
				}
				continue
			}
			if under(d, zdir) && (name == ".env" || strings.ContainsAny(name, "*?[")) {
				return true
			}
		}
	}
	return false
}

// expandBraces expands a word the way the shell's brace expansion would —
// comma lists and {a..z}/{1..9} ranges, nested — up to maxBraceWords results.
func expandBraces(w string) []string {
	out := []string{}
	var walk func(string)
	walk = func(s string) {
		if len(out) >= maxBraceWords {
			return
		}
		open, close, alts := braceGroup(s)
		if open < 0 {
			out = append(out, s)
			return
		}
		for _, a := range alts {
			walk(s[:open] + a + s[close+1:])
		}
	}
	walk(w)
	return out
}

const maxBraceWords = 256

// braceGroup finds the first {…} in s that expands (a top-level comma or a
// range) and returns its bounds and alternatives; open < 0 when there is none.
func braceGroup(s string) (open, close int, alts []string) {
	for i := 0; i < len(s); i++ {
		if s[i] != '{' {
			continue
		}
		depth, start := 0, i+1
		var parts []string
		for j := i; j < len(s); j++ {
			switch s[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					parts = append(parts, s[start:j])
					if len(parts) > 1 {
						return i, j, parts
					}
					if r := braceRange(parts[0]); r != nil {
						return i, j, r
					}
					j = len(s) // not an expansion: look for the next {
				}
			case ',':
				if depth == 1 {
					parts = append(parts, s[start:j])
					start = j + 1
				}
			}
		}
	}
	return -1, -1, nil
}

var reBraceRange = regexp.MustCompile(`^(-?\d+|[a-zA-Z])\.\.(-?\d+|[a-zA-Z])$`)

// braceRange expands `a..z` or `1..9`; nil when body is not a range.
func braceRange(body string) []string {
	m := reBraceRange.FindStringSubmatch(body)
	if m == nil {
		return nil
	}
	var r []string
	if x, y, ok := atoi2(m[1], m[2]); ok {
		for i, step := x, sign(y-x); len(r) < maxBraceWords; i += step {
			r = append(r, strconv.Itoa(i))
			if i == y {
				break
			}
		}
		return r
	}
	if len(m[1]) == 1 && len(m[2]) == 1 && !isDigit(m[1][0]) && !isDigit(m[2][0]) {
		x, y := m[1][0], m[2][0]
		for c := x; ; {
			r = append(r, string(c))
			if c == y {
				break
			}
			if y > x {
				c++
			} else {
				c--
			}
		}
		return r
	}
	return nil
}

func atoi2(a, b string) (int, int, bool) {
	x, err1 := strconv.Atoi(a)
	y, err2 := strconv.Atoi(b)
	return x, y, err1 == nil && err2 == nil
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// canonical cleans path and resolves symlinks in its longest existing prefix,
// so a missing tail (a glob or brace word) still compares on the real directory.
func canonical(path string) string {
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	if dir := filepath.Dir(path); dir != path {
		return filepath.Join(canonical(dir), filepath.Base(path))
	}
	return path
}

var reRawPushF = regexp.MustCompile(`push.* -f( |$)`)

// dangerousRaw: R7 keywords in the raw command (quotes dropped, lower-cased).
func dangerousRaw(raw string) bool {
	s := strings.ToLower(quoteStripper.Replace(raw))
	for _, k := range []string{"pr merge", "/merge", "--force", ".zenify/.env",
		"stack merge", "stack link", "stack unstack", "stack delete", "stack alias", "gh-stack ", "/unstack"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return reRawPushF.MatchString(s)
}
