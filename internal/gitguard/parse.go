package gitguard

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// GitCall is one `git` invocation parsed from the AST.
type GitCall struct {
	Sub       string   // the real subcommand (e.g. "commit","push","merge")
	Args      []string // literal tokens after the subcommand
	RepoFlagC string   // value of -C <dir> if present, empty otherwise
	alias     *GitCall // the call with a `-c alias.<Sub>=…` substituted, nil when none
}

// isGitBinary accepts "git" or a path ending in "/git".
func isGitBinary(tok string) bool {
	return tok == "git" || strings.HasSuffix(tok, "/git")
}

// parseGitCall builds a GitCall from the Args of a CallExpr already known to
// call git. Skips global options and their option args (keeps the same
// semantics as the bash guard).
func parseGitCall(args []string) (GitCall, bool) {
	// args[0] is "git" or "*/git" (wrappers such as env are already stripped
	// by Parse; assignment prefixes are not part of Args).
	i := 1
	gc := GitCall{}
	aliases := map[string]string{} // -c alias.<name>=<value>, name lower-cased
	for i < len(args) {
		tok := args[i]
		switch {
		case tok == "-C" || tok == "-c" || tok == "--git-dir" ||
			tok == "--work-tree" || tok == "--namespace" || tok == "--exec-path":
			// Only consume this flag's arg if there's still room for a token
			// after it — avoids swallowing the real subcommand on a
			// degenerate repeated -C (e.g. "git -C -C -C push" must
			// recognize "push", not swallow it as the value of the last -C).
			if i+2 < len(args) {
				if tok == "-C" {
					gc.RepoFlagC = args[i+1]
				}
				if key, val, ok := strings.Cut(args[i+1], "="); tok == "-c" && ok &&
					strings.HasPrefix(strings.ToLower(key), "alias.") {
					aliases[strings.ToLower(key[len("alias."):])] = val
				}
				i += 2 // skip the flag + its arg
				continue
			}
			i++
			continue
		case strings.HasPrefix(tok, "-"):
			i++ // other global flags take no arg
			continue
		default:
			gc.Sub = tok
			gc.Args = args[i+1:]
			// A non-shell alias runs its first word as the subcommand. Git
			// ignores an alias that shadows a built-in, so both are kept.
			if f := strings.Fields(aliases[strings.ToLower(tok)]); len(f) > 0 && !strings.HasPrefix(f[0], "!") {
				gc.alias = &GitCall{Sub: f[0], Args: append(f[1:], gc.Args...), RepoFlagC: gc.RepoFlagC}
			}
			return gc, true
		}
	}
	return gc, false // no real subcommand
}

// maxNestDepth is how many nested `bash -c`/`eval` strings Parse follows;
// a deeper one makes the command Unparseable (FR-1.2).
const maxNestDepth = 3

// Cmd is one simple command after wrapper stripping.
type Cmd struct {
	Argv         []string // word values per FR-1.6; "" for a word with an expansion
	Src          []string // source text of each word, quotes removed
	PRDExpansion bool     // some word expands a parameter whose name ends in _PRD
	// RedirArgv/RedirSrc: input-redirect words (<, <>, <&, <<<) of the
	// statement, kept apart from Argv so git parsing never sees them;
	// RedirSrc also holds heredoc bodies.
	RedirArgv, RedirSrc []string
}

// redirectCmd is Argv[0] of the Cmd that carries the input redirects of a
// statement that is not a simple command (a loop, a subshell, a bare `< f`).
const redirectCmd = "<redirect>"

// Parsed is every simple command found in a Bash command string, including
// those inside $(…), subshells and nested `bash -c`/`eval` strings.
type Parsed struct {
	Cmds        []Cmd
	Unparseable bool // strict re-parse failed, nesting > 3, or a dangerous git alias (FR-1.5)
	// PRDExpansion: a _PRD parameter expansion anywhere in the AST at any
	// nesting level (assignments, declarations, redirects); Task 2's R5 checks it.
	PRDExpansion bool
}

// Parse parses command into its simple commands. A recovering parser yields
// the best-effort Cmds; a strict parser decides Unparseable, because the
// recovering one returns no error on an unclosed quote.
func Parse(command string) Parsed {
	var p Parsed
	parseInto(command, 0, &p)
	return p
}

func parseInto(src string, depth int, p *Parsed) {
	if depth > maxNestDepth {
		p.Unparseable = true
		return
	}
	if _, err := syntax.NewParser().Parse(strings.NewReader(src), ""); err != nil {
		p.Unparseable = true
	}
	file, err := syntax.NewParser(syntax.RecoverErrors(4)).Parse(strings.NewReader(src), "")
	if file == nil && err != nil {
		return
	}
	if hasPRDExpansion(file) {
		p.PRDExpansion = true
	}
	redirs := map[*syntax.CallExpr][]*syntax.Redirect{}
	syntax.Walk(file, func(node syntax.Node) bool {
		if st, ok := node.(*syntax.Stmt); ok {
			if ce, ok := st.Cmd.(*syntax.CallExpr); ok && len(ce.Args) > 0 {
				redirs[ce] = st.Redirs
			} else if c := addRedirs(Cmd{Argv: []string{redirectCmd}, Src: []string{redirectCmd}}, src, st.Redirs); len(c.RedirSrc) > 0 {
				p.Cmds = append(p.Cmds, c)
			}
			return true
		}
		ce, ok := node.(*syntax.CallExpr)
		if !ok || len(ce.Args) == 0 {
			return true
		}
		cmd := Cmd{Argv: make([]string, len(ce.Args)), Src: make([]string, len(ce.Args))}
		for i, w := range ce.Args {
			cmd.Argv[i] = wordValue(w)
			cmd.Src[i] = wordSrc(src, w)
			cmd.PRDExpansion = cmd.PRDExpansion || hasPRDExpansion(w)
		}
		for _, a := range ce.Assigns {
			if a.Value != nil && hasPRDExpansion(a.Value) {
				cmd.PRDExpansion = true
			}
		}
		cmd = addRedirs(stripWrappers(cmd), src, redirs[ce])
		p.Cmds = append(p.Cmds, cmd)
		if isGitBinary(cmd.Argv[0]) && hasDangerousAlias(cmd.Argv) {
			p.Unparseable = true
		}
		if script, dynamic := nestedScript(cmd.Argv); dynamic {
			p.Unparseable = true
		} else if script != "" {
			parseInto(script, depth+1, p)
		}
		return true
	})
}

// addRedirs appends the input redirects among rs to c.
func addRedirs(c Cmd, src string, rs []*syntax.Redirect) Cmd {
	for _, r := range rs {
		switch r.Op {
		case syntax.RdrIn, syntax.RdrInOut, syntax.DplIn, syntax.WordHdoc:
			c.RedirArgv = append(c.RedirArgv, wordValue(r.Word))
			c.RedirSrc = append(c.RedirSrc, wordSrc(src, r.Word))
		case syntax.Hdoc, syntax.DashHdoc:
			if r.Hdoc != nil {
				c.RedirSrc = append(c.RedirSrc, wordSrc(src, r.Hdoc))
			}
		}
	}
	return c
}

// wordValue returns the word's value when it is made only of literals and
// quotes without expansions (FR-1.6), "" otherwise.
func wordValue(w *syntax.Word) string {
	var b strings.Builder
	for _, part := range w.Parts {
		switch x := part.(type) {
		case *syntax.Lit:
			b.WriteString(unescape(x.Value, ""))
		case *syntax.SglQuoted:
			if x.Dollar {
				return ""
			}
			b.WriteString(x.Value)
		case *syntax.DblQuoted:
			if x.Dollar {
				return ""
			}
			for _, in := range x.Parts {
				lit, ok := in.(*syntax.Lit)
				if !ok {
					return ""
				}
				b.WriteString(unescape(lit.Value, "\"\\$`"))
			}
		default:
			return ""
		}
	}
	return b.String()
}

// unescape drops a backslash before any character (only == "") or before a
// character listed in only.
func unescape(s, only string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && (only == "" || strings.IndexByte(only, s[i+1]) >= 0) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

var quoteStripper = strings.NewReplacer(`'`, "", `"`, "")

// wordSrc is the word's source text with ' and " removed.
func wordSrc(src string, w *syntax.Word) string {
	s, e := w.Pos().Offset(), w.End().Offset()
	if !w.Pos().IsValid() || s > e || e > uint(len(src)) {
		return ""
	}
	return quoteStripper.Replace(src[s:e])
}

// hasPRDExpansion reports a parameter expansion of a name ending in _PRD
// anywhere under n ($X_PRD, ${X_PRD}, ${X_PRD:-…}).
func hasPRDExpansion(n syntax.Node) bool {
	found := false
	syntax.Walk(n, func(node syntax.Node) bool {
		if pe, ok := node.(*syntax.ParamExp); ok && pe.Param != nil && strings.HasSuffix(pe.Param.Value, "_PRD") {
			found = true
		}
		return !found
	})
	return found
}

func basename(tok string) string {
	return tok[strings.LastIndexByte(tok, '/')+1:]
}

// stripWrappers drops leading wrapper commands (FR-1.4) until Argv[0] is not
// one. A wrapper with nothing after it is left as is.
func stripWrappers(c Cmd) Cmd {
	for {
		n := wrapperLen(c.Argv)
		if n == 0 || n >= len(c.Argv) {
			return c
		}
		c.Argv, c.Src = c.Argv[n:], c.Src[n:]
	}
}

// wrapperLen is how many leading tokens the wrapper in argv[0] consumes, 0
// when argv[0] is not a wrapper.
func wrapperLen(argv []string) int {
	switch basename(argv[0]) {
	case "command", "exec", "nohup":
		return 1
	case "time":
		return skipFlags(argv, nil)
	case "env":
		i := 1
		for i < len(argv) {
			switch t := argv[i]; {
			case t == "-u" || t == "-C":
				i += 2
			case strings.HasPrefix(t, "-") || strings.Contains(t, "="):
				i++
			default:
				return i
			}
		}
		return i
	case "nice":
		return skipFlags(argv, []string{"-n"})
	case "timeout":
		return skipFlags(argv, []string{"-s", "-k"}) + 1 // + the duration
	case "sudo":
		return skipFlags(argv, []string{"-u", "-g", "-C", "-D", "-h", "-p", "-r", "-t", "-T", "-U"})
	case "xargs":
		return skipFlags(argv, []string{"-I", "-n", "-P", "-L", "-d", "-E", "-s", "-a"})
	}
	return 0
}

// skipFlags returns the index of the first token after argv[0] that is not a
// flag; flags in withArg also consume the token after them.
func skipFlags(argv, withArg []string) int {
	i := 1
	for i < len(argv) && strings.HasPrefix(argv[i], "-") {
		if slices.Contains(withArg, argv[i]) {
			i++
		}
		i++
	}
	return i
}

// nestedScript returns the command string run by `bash|sh|zsh … -c <s>` or
// `eval <args…>` when every part of it has a value (FR-1.1). dynamic is true
// when that string has an expansion, or the shell reads its script from stdin
// (no -c and no script operand, or -s): the script cannot be known here.
func nestedScript(argv []string) (script string, dynamic bool) {
	switch basename(argv[0]) {
	case "bash", "sh", "zsh":
		stdin := false
		for i := 1; i < len(argv); i++ {
			t := argv[i]
			if t == "--" {
				return "", stdin || i+1 == len(argv)
			}
			if !strings.HasPrefix(t, "-") && !strings.HasPrefix(t, "+") {
				return "", stdin // a script-file operand, unless -s
			}
			if strings.HasPrefix(t, "--") {
				continue
			}
			if strings.HasPrefix(t, "-") && strings.Contains(t, "c") {
				for i++; i < len(argv); i++ {
					if t := argv[i]; t == "" || !strings.HasPrefix(t, "-") {
						return t, t == ""
					}
				}
				return "", false
			}
			if strings.HasPrefix(t, "-") && strings.Contains(t, "s") {
				stdin = true
			}
			if strings.HasSuffix(t, "o") {
				i++ // -o/+o (or -euo) takes an option name
			}
		}
		return "", true // no -c and no operand: the script comes from stdin
	case "eval":
		if len(argv) < 2 {
			return "", false
		}
		if slices.Contains(argv[1:], "") {
			return "", true
		}
		return strings.Join(argv[1:], " "), false
	}
	return "", false
}

// gitArgFlags are git global options, other than -c, that take an argument.
var gitArgFlags = []string{"-C", "--git-dir", "--work-tree", "--namespace", "--exec-path"}

// hasDangerousAlias reports `git -c alias.<x>=<v>` among git's global options
// whose value runs push, merge or commit (FR-1.5).
func hasDangerousAlias(argv []string) bool {
	for i := 1; i+1 < len(argv) && strings.HasPrefix(argv[i], "-"); i++ {
		if argv[i] != "-c" {
			if slices.Contains(gitArgFlags, argv[i]) {
				i++
			}
			continue
		}
		i++
		key, val, _ := strings.Cut(argv[i], "=")
		if strings.HasPrefix(strings.ToLower(key), "alias.") &&
			(strings.Contains(val, "push") || strings.Contains(val, "merge") || strings.Contains(val, "commit")) {
			return true
		}
	}
	return false
}

// ParseGitCalls returns every real git call in the command, built from Parse
// (so wrappers and nested `bash -c`/`eval` strings are seen through).
func ParseGitCalls(command string) []GitCall {
	return gitCalls(Parse(command))
}

func gitCalls(p Parsed) []GitCall {
	var calls []GitCall
	for _, c := range p.Cmds {
		if !isGitBinary(c.Argv[0]) {
			continue
		}
		calls = append(calls, gitCallVariants(c.Argv)...)
	}
	return calls
}

// gitCallVariants is the git call in argv as written and, when a -c alias
// names its subcommand, as substituted; a rule denies if either does.
func gitCallVariants(argv []string) []GitCall {
	gc, ok := parseGitCall(argv)
	if !ok {
		return nil
	}
	if gc.alias != nil {
		return []GitCall{gc, *gc.alias}
	}
	return []GitCall{gc}
}

// LeadingCd extracts the target of a `cd <dir>` that runs first: the first
// command of the first statement, or the head of a leading &&/|| chain, looking
// into a leading ( … ) or { …; } group. A cd
// anywhere else runs after other commands and does not re-anchor them.
func LeadingCd(command string) string {
	parser := syntax.NewParser(syntax.RecoverErrors(4))
	file, _ := parser.Parse(strings.NewReader(command), "")
	if file == nil || len(file.Stmts) == 0 {
		return ""
	}
	cmd := file.Stmts[0].Cmd
	for {
		switch x := cmd.(type) {
		case *syntax.BinaryCmd:
			if x.Op != syntax.AndStmt && x.Op != syntax.OrStmt {
				return ""
			}
			cmd = x.X.Cmd
			continue
		case *syntax.Subshell:
			if len(x.Stmts) > 0 {
				cmd = x.Stmts[0].Cmd
				continue
			}
		case *syntax.Block:
			if len(x.Stmts) > 0 {
				cmd = x.Stmts[0].Cmd
				continue
			}
		}
		break
	}
	ce, ok := cmd.(*syntax.CallExpr)
	if !ok || len(ce.Args) < 2 || ce.Args[0].Lit() != "cd" {
		return ""
	}
	return ce.Args[1].Lit()
}
