package gitguard

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// GitCall is one `git` invocation parsed from the AST.
type GitCall struct {
	Sub       string   // the real subcommand (e.g. "commit","push","merge")
	Args      []string // literal tokens after the subcommand
	RepoFlagC string   // value of -C <dir> if present, empty otherwise
}

// litWords converts []*syntax.Word into []string literals; a word that isn't
// pure literal (substitution, variable) returns "" — safe because we only
// match against already-known literals.
func litWords(words []*syntax.Word) []string {
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = w.Lit()
	}
	return out
}

// isGitBinary accepts "git" or a path ending in "/git".
func isGitBinary(tok string) bool {
	return tok == "git" || strings.HasSuffix(tok, "/git")
}

// parseGitCall builds a GitCall from the Args of a CallExpr already known to
// call git. Skips global options and their option args (keeps the same
// semantics as the bash guard).
func parseGitCall(args []string) (GitCall, bool) {
	// args[0] is "git" or "*/git" (env-prefix already filtered by the caller
	// if the CallExpr uses Assigns; env-as-word is handled by the caller).
	i := 1
	gc := GitCall{}
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
			return gc, true
		}
	}
	return gc, false // no real subcommand
}

// ParseGitCalls parses the command into an AST and returns every real git
// call. RecoverErrors so it doesn't break on a malformed command; parse
// errors → best-effort.
func ParseGitCalls(command string) []GitCall {
	parser := syntax.NewParser(syntax.RecoverErrors(4))
	file, err := parser.Parse(strings.NewReader(command), "")
	if file == nil && err != nil {
		return nil
	}
	var calls []GitCall
	syntax.Walk(file, func(node syntax.Node) bool {
		ce, ok := node.(*syntax.CallExpr)
		if !ok || len(ce.Args) == 0 {
			return true
		}
		words := litWords(ce.Args)
		// "env VAR=val ..." prefix as a word: skip "env" + tokens containing
		// '=' until "git" is reached.
		start := 0
		if words[0] == "env" {
			start = 1
			for start < len(words) && strings.Contains(words[start], "=") {
				start++
			}
		}
		if start >= len(words) || !isGitBinary(words[start]) {
			return true
		}
		if gc, ok := parseGitCall(words[start:]); ok {
			calls = append(calls, gc)
		}
		return true
	})
	return calls
}

// LeadingCd extracts the target of a leading `cd <dir>` in the command (if any).
func LeadingCd(command string) string {
	parser := syntax.NewParser(syntax.RecoverErrors(4))
	file, err := parser.Parse(strings.NewReader(command), "")
	if file == nil && err != nil {
		return ""
	}
	var target string
	syntax.Walk(file, func(node syntax.Node) bool {
		if target != "" {
			return false
		}
		ce, ok := node.(*syntax.CallExpr)
		if !ok || len(ce.Args) < 2 {
			return true
		}
		if ce.Args[0].Lit() == "cd" {
			if d := ce.Args[1].Lit(); d != "" {
				target = d
				return false
			}
		}
		return true
	})
	return target
}
