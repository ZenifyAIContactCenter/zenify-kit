package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var histT0 = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

type rec map[string]any

func ts(sec int) string {
	return histT0.Add(time.Duration(sec) * time.Second).Format("2006-01-02T15:04:05.000Z")
}

func prompt(cwd, branch string, sec int, text string) rec {
	return rec{"type": "user", "cwd": cwd, "gitBranch": branch, "timestamp": ts(sec), "message": rec{"role": "user", "content": text}}
}

func toolResult(cwd, branch string, sec int, id string) rec {
	return rec{"type": "user", "cwd": cwd, "gitBranch": branch, "timestamp": ts(sec), "message": rec{"role": "user", "content": []any{rec{"type": "tool_result", "tool_use_id": id, "content": "ok"}}}}
}

func assistant(cwd, branch string, sec int, mid string, tool rec, usage bool) rec {
	content := []any{rec{"type": "text", "text": "hi"}}
	if tool != nil {
		content = []any{tool}
	}
	m := rec{"id": mid, "model": "opus", "role": "assistant", "content": content}
	if usage {
		m["usage"] = rec{"input_tokens": 100, "output_tokens": 10, "cache_read_input_tokens": 9999}
	}
	return rec{"type": "assistant", "cwd": cwd, "gitBranch": branch, "timestamp": ts(sec), "message": m}
}

func writeJSONL(t *testing.T, path string, recs []rec, extraLines ...string) {
	t.Helper()
	var sb strings.Builder
	for _, r := range recs {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	for _, l := range extraLines {
		sb.WriteString(l + "\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func histEnv(t *testing.T) (ws, projects string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return trackWS(t, true), filepath.Join(home, ".claude", "projects", "p1")
}

func scFixture(ws string) []rec {
	w := filepath.Join(ws, "repos", "kit", ".worktrees", "x")
	mk := func(r rec, skill bool) rec {
		if skill {
			r["attributionSkill"] = "znf:fix"
		}
		return r
	}
	use := func(name, id string) rec { return rec{"type": "tool_use", "id": id, "name": name} }
	return []rec{
		// Outside the workspace: dropped.
		assistant("/elsewhere/repos/kit", "u/feat/z", 0, "m9", nil, true),
		mk(prompt(w, "HEAD", 0, "fix it"), false),
		mk(assistant(w, "HEAD", 120, "m1", use("Bash", "t1"), true), true),
		// A meta record and a task notification between prompt and assistant are not prompts.
		func() rec { r := prompt(w, "HEAD", 180, "meta"); r["isMeta"] = true; return r }(),
		prompt(w, "HEAD", 210, "<task-notification>done</task-notification>"),
		toolResult(w, "HEAD", 240, "t1"),
		mk(assistant(w, "HEAD", 300, "m1", nil, true), false),
		prompt(w, "HEAD", 1800, "next"),
		assistant(w, "HEAD", 1860, "m2", use("AskUserQuestion", "t2"), false),
		toolResult(w, "HEAD", 2400, "t2"),
		assistant(w, "HEAD", 2520, "m3", nil, false),
	}
}

// SC-33
func TestTrackOnboardHistory_PreviewAndWrite(t *testing.T) {
	ws, proj := histEnv(t)
	writeJSONL(t, filepath.Join(proj, "s1.jsonl"), scFixture(ws), "{broken json")
	out, err := runTrack(t, ws, "onboard", "history")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "kit") || !strings.Contains(out, "fix") || !strings.Contains(out, "8m") || !strings.Contains(out, "opus:100/10") {
		t.Fatalf("preview = %q", out)
	}
	if strings.Contains(out, "elsewhere") || strings.Count(out, "\n") > 4 {
		t.Fatalf("preview has unexpected groups: %q", out)
	}
	if !strings.Contains(out, "skipped 1") {
		t.Fatalf("preview lacks skipped count: %q", out)
	}
	if trackLog(t, ws) != "" {
		t.Fatal("preview wrote the log")
	}
	for i := 0; i < 2; i++ {
		if _, err := runTrack(t, ws, "onboard", "history", "--write"); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(strings.TrimSpace(trackLog(t, ws)), "\n")
	if len(lines) != 1 || !strings.HasSuffix(lines[0], " · history · 67ce47d0 · fix · kit · 8 · opus:100/10") {
		t.Fatalf("log = %q", trackLog(t, ws))
	}
	if out, err := runTrack(t, ws, "stats", "--json"); err != nil || strings.Contains(out, "malformed\": 1") {
		t.Fatalf("history row must parse: %q %v", out, err)
	}
}

func TestTrackOnboardHistory_MainCheckoutBranchAndSince(t *testing.T) {
	ws, proj := histEnv(t)
	hub := filepath.Join(ws, "repos", "hub")
	writeJSONL(t, filepath.Join(proj, "s2.jsonl"), []rec{
		prompt(hub, "u/feat/y", 0, "go"),
		assistant(hub, "u/feat/y", 61, "a1", nil, true),
		// Main checkout on a branch that does not match: no group.
		prompt(hub, "main", 100, "ignored"),
		assistant(hub, "main", 400, "a2", nil, true),
	})
	out, err := runTrack(t, ws, "onboard", "history")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hub") || !strings.Contains(out, " y ") || !strings.Contains(out, "cook") || !strings.Contains(out, "2m") {
		t.Fatalf("preview = %q", out)
	}
	if strings.Contains(out, "main") {
		t.Fatalf("main branch leaked: %q", out)
	}
	out, err = runTrack(t, ws, "onboard", "history", "--since", "2026-10-02")
	if err != nil || strings.Contains(out, "hub") {
		t.Fatalf("--since must drop earlier records: %q %v", out, err)
	}
	if _, err := runTrack(t, ws, "onboard", "history", "--since", "yesterday"); err == nil {
		t.Fatal("bad --since must fail")
	}
}

func TestTrackOnboardHistory_SubagentFileAndKindFallback(t *testing.T) {
	ws, proj := histEnv(t)
	w := filepath.Join(ws, "repos", "kit", ".worktrees", "z")
	// The first line of a subagent file is the dispatch text, a prompt even when it looks like a notification.
	writeJSONL(t, filepath.Join(proj, "sess", "subagents", "agent-1.jsonl"), []rec{
		prompt(w, "HEAD", 0, "<system-reminder>dispatch</system-reminder>"),
		assistant(w, "HEAD", 180, "s1", nil, true),
	})
	out, err := runTrack(t, ws, "onboard", "history")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "3m") || !strings.Contains(out, " - ") {
		t.Fatalf("preview = %q", out)
	}
}

func TestTrackOnboardHistory_NoTranscripts(t *testing.T) {
	ws, _ := histEnv(t)
	if out, err := runTrack(t, ws, "onboard", "history"); err != nil || !strings.Contains(out, "skipped 0") {
		t.Fatalf("%q %v", out, err)
	}
}
