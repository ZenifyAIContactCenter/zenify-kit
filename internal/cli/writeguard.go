package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

// rootAllowlist is the set of workspace-root entries an agent may write to.
var rootAllowlist = map[string]bool{"docs": true, "CLAUDE.md": true, "AGENTS.md": true, ".claude": true, ".zenify": true, ".zenify-overlay.yaml": true, ".git": true, ".idea": true, ".vscode": true, ".DS_Store": true, ".agents": true, ".codex": true}

// writeGuardPayload is the PreToolUse stdin shape for Write|Edit|NotebookEdit.
type writeGuardPayload struct {
	ToolName  string `json:"tool_name"`
	Cwd       string `json:"cwd"`
	ToolInput struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
}

const writeGuardDeny = "write-guard: %s is outside the workspace layout. temp → $TMPDIR · research → docs/research/<repo>/ · notes → docs/notes/ · script for the user → docs/scripts/<repo>/ · deliverable → docs/exports/ · external document → docs/external/ · code → repos/<repo>/.worktrees/<slug>/"

// runWriteGuard is the exit-code core of the write-guard PreToolUse hook,
// shaped like runReadGuard: deny = message on stderr + exit 2, everything else
// (bad JSON, no workspace, panic anywhere) = exit 0 and silence.
func runWriteGuard(stdin io.Reader, stderr io.Writer, findWorkspace func(string) (string, bool)) (code int) {
	defer func() {
		if r := recover(); r != nil {
			code = 0
		}
	}()
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return 0
	}
	var p writeGuardPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return 0
	}
	path := p.ToolInput.FilePath
	if path == "" {
		path = p.ToolInput.NotebookPath
	}
	if path == "" {
		return 0
	}
	if !filepath.IsAbs(path) && p.Cwd != "" {
		path = filepath.Join(p.Cwd, path)
	}
	path = filepath.Clean(path)
	ws, ok := findWorkspace(filepath.Dir(path))
	if !ok {
		return 0
	}
	rel, err := filepath.Rel(ws, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return 0
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	deny := func() int {
		_, _ = fmt.Fprintf(stderr, writeGuardDeny+"\n", path)
		return 2
	}
	switch {
	case repoRootSegs(ws)[segs[0]]:
		return 0
	case !rootAllowlist[segs[0]]:
		return deny()
	case segs[0] == "docs":
		if len(segs) < 3 { // a file directly under docs/
			return deny()
		}
		if strings.HasPrefix(segs[1], ".") {
			return 0
		}
		if !slices.Contains(docsDirs, segs[1]) {
			return deny()
		}
	}
	return 0
}
