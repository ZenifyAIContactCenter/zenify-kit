package gitguard

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// GitCall là một lời gọi `git` đã parse từ AST.
type GitCall struct {
	Sub       string   // subcommand thật (vd "commit","push","merge")
	Args      []string // các token literal sau subcommand
	RepoFlagC string   // giá trị của -C <dir> nếu có, rỗng nếu không
}

// litWords chuyển []*syntax.Word thành []string literal; word không thuần
// literal (subst, biến) trả "" — an toàn vì ta chỉ so khớp literal đã biết.
func litWords(words []*syntax.Word) []string {
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = w.Lit()
	}
	return out
}

// isGitBinary nhận "git" hoặc đường dẫn kết thúc "/git".
func isGitBinary(tok string) bool {
	return tok == "git" || strings.HasSuffix(tok, "/git")
}

// parseGitCall dựng GitCall từ Args của một CallExpr đã biết là gọi git.
// Bỏ qua global option và arg của option (giữ đúng semantics guard bash).
func parseGitCall(args []string) (GitCall, bool) {
	// args[0] là "git" hoặc "*/git" (đã lọc env-prefix ở caller nếu CallExpr
	// dùng Assigns; env-as-word xử lý ở caller).
	i := 1
	gc := GitCall{}
	for i < len(args) {
		tok := args[i]
		switch {
		case tok == "-C" || tok == "-c" || tok == "--git-dir" ||
			tok == "--work-tree" || tok == "--namespace" || tok == "--exec-path":
			// Chỉ ăn arg của flag này nếu vẫn còn chỗ cho một token sau đó —
			// tránh nuốt mất subcommand thật khi gặp -C lặp lại thoái hoá
			// (vd "git -C -C -C push" phải nhận ra "push", không phải nuốt
			// nó làm giá trị của -C cuối cùng).
			if i+2 < len(args) {
				if tok == "-C" {
					gc.RepoFlagC = args[i+1]
				}
				i += 2 // bỏ flag + arg của nó
				continue
			}
			i++
			continue
		case strings.HasPrefix(tok, "-"):
			i++ // global flag khác không nhận arg
			continue
		default:
			gc.Sub = tok
			gc.Args = args[i+1:]
			return gc, true
		}
	}
	return gc, false // không có subcommand thật
}

// ParseGitCalls parse command thành AST và trả mọi lời gọi git thật.
// RecoverErrors để không gãy trên lệnh dở; lỗi parse → best-effort.
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
		// Tiền tố "env VAR=val ..." dạng word: bỏ "env" + các token có '='
		// tới khi gặp "git".
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

// LeadingCd trích đích của `cd <dir>` dẫn đầu command (nếu có).
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
