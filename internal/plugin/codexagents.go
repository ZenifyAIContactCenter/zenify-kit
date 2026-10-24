package plugin

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CodexAgentMarker is the first line of every Codex agent file the kit writes;
// files without it are the user's and are never touched.
const CodexAgentMarker = "# managed by zenify — do not edit"

// AgentSyncResult lists agent names by outcome.
type AgentSyncResult struct{ Written, Unchanged, Skipped, Removed []string }

type agentMeta struct{ Name, Description, Tools, DisallowedTools string }

// parseAgentFrontmatter is a line-based parser in the same style as
// docsgen.ParseFrontmatter (split on the first ':'); plugin cannot import
// docsgen because docsgen's tests import plugin.
func parseAgentFrontmatter(b []byte) (agentMeta, string, error) {
	var m agentMeta
	delim := []byte("---\n")
	if !bytes.HasPrefix(b, delim) {
		return m, "", errors.New("no frontmatter")
	}
	rest := b[len(delim):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return m, "", errors.New("unterminated frontmatter")
	}
	sc := bufio.NewScanner(bytes.NewReader(rest[:end]))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		raw := sc.Text()
		idx := strings.Index(raw, ":")
		if idx < 0 {
			continue
		}
		val := strings.TrimSpace(raw[idx+1:])
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		switch raw[:idx] {
		case "name":
			m.Name = val
		case "description":
			m.Description = val
		case "tools":
			m.Tools = val
		case "disallowedTools":
			m.DisallowedTools = val
		}
	}
	body := rest[end+len("\n---"):]
	if i := bytes.IndexByte(body, '\n'); i >= 0 {
		body = body[i+1:]
	} else {
		body = nil
	}
	return m, string(body), sc.Err()
}

func agentTOML(name string, fm agentMeta, body string) (string, error) {
	if strings.Contains(body, "'''") {
		return "", fmt.Errorf("agent %s: body contains '''", name)
	}
	effort := "medium"
	if name == "spec-reviewer" {
		effort = "high"
	}
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	var sb strings.Builder
	sb.WriteString(CodexAgentMarker + "\n")
	fmt.Fprintf(&sb, "name = \"%s\"\n", esc.Replace(name))
	fmt.Fprintf(&sb, "description = \"%s\"\n", esc.Replace(fm.Description))
	fmt.Fprintf(&sb, "model_reasoning_effort = \"%s\"\n", effort)
	if strings.Contains(fm.DisallowedTools, "Write") && strings.Contains(fm.DisallowedTools, "Edit") {
		sb.WriteString("sandbox_mode = \"read-only\"\n")
	}
	// No mcp_servers key: real Codex 0.155.1 requires a table (name -> server config),
	// not the list the plan sketched, and rejects the whole file otherwise. A ui-verifier
	// drives the browser through `zenify browser` (Bash), so it needs no MCP server.
	sb.WriteString("developer_instructions = '''\n")
	sb.WriteString(strings.TrimLeft(body, "\n"))
	if !strings.HasSuffix(body, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("'''\n")
	return sb.String(), nil
}

func hasMarker(p string) (exists, marked bool) {
	b, err := os.ReadFile(p) //nolint:gosec // G304 -- path built from codexHome internally
	if err != nil {
		return false, false
	}
	first, _, _ := strings.Cut(string(b), "\n")
	return true, first == CodexAgentMarker
}

// SyncCodexAgents writes each embedded agent as <codexHome>/agents/<name>.toml
// and removes marked files for agents the kit no longer ships.
// A file lacking the marker line is the user's: left alone and reported in Skipped.
func SyncCodexAgents(codexHome string, dryRun bool) (AgentSyncResult, error) {
	var res AgentSyncResult
	entries, err := fs.ReadDir(assets, embedRoot+"/agents")
	if err != nil {
		return res, err
	}
	dir := filepath.Join(codexHome, "agents")
	shipped := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		shipped[name] = true
		src, err := assets.ReadFile(embedRoot + "/agents/" + e.Name())
		if err != nil {
			return res, err
		}
		fm, body, err := parseAgentFrontmatter(src)
		if err != nil {
			return res, fmt.Errorf("agent %s: %w", name, err)
		}
		out, err := agentTOML(name, fm, body)
		if err != nil {
			return res, err
		}
		target := filepath.Join(dir, name+".toml")
		if exists, marked := hasMarker(target); exists {
			if !marked {
				res.Skipped = append(res.Skipped, name)
				continue
			}
			if cur, _ := os.ReadFile(target); string(cur) == out { //nolint:gosec // G304 -- internal path
				res.Unchanged = append(res.Unchanged, name)
				continue
			}
		}
		if !dryRun {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return res, err
			}
			if err := os.WriteFile(target, []byte(out), 0o600); err != nil {
				return res, err
			}
		}
		res.Written = append(res.Written, name)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.toml"))
	if err != nil {
		return res, err
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".toml")
		if shipped[name] {
			continue
		}
		if _, marked := hasMarker(f); !marked {
			continue
		}
		if !dryRun {
			if err := os.Remove(f); err != nil {
				return res, err
			}
		}
		res.Removed = append(res.Removed, name)
	}
	return res, nil
}

// RemoveCodexAgents deletes only the agents/*.toml files that carry the marker line.
func RemoveCodexAgents(codexHome string, dryRun bool) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(codexHome, "agents", "*.toml"))
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, f := range files {
		if _, marked := hasMarker(f); !marked {
			continue
		}
		if !dryRun {
			if err := os.Remove(f); err != nil {
				return removed, err
			}
		}
		removed = append(removed, strings.TrimSuffix(filepath.Base(f), ".toml"))
	}
	sort.Strings(removed)
	return removed, nil
}
