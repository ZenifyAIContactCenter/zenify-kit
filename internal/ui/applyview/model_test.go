package applyview

import (
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
	"github.com/charmbracelet/lipgloss"
)

func TestModel_ProgressAdvancesBarAndList(t *testing.T) {
	m := New(3)

	// First repo done.
	nm, _ := m.Update(ProgressMsg{Done: 1, Total: 3, Repo: "repo-a", State: reconcile.OK})
	m = nm.(Model)
	// Second repo in flight.
	nm, _ = m.Update(ProgressMsg{Done: 2, Total: 3, Repo: "repo-b", State: reconcile.OK})
	m = nm.(Model)

	view := m.View()
	if !strings.Contains(view, "2/3") {
		t.Fatalf("view must show 2/3 progress, got:\n%s", view)
	}
	if !strings.Contains(view, "repo-a") {
		t.Fatalf("view must list the finished repo-a, got:\n%s", view)
	}
	// The bar must have advanced: at least one filled cell.
	if !strings.Contains(view, "▰") {
		t.Fatalf("view must show a filled bar cell, got:\n%s", view)
	}
}

// TestModel_FailedRepoMarksDistinctly binds FR-1.2: a failed repo must render
// with a distinct mark (✗) from a successful one (✓), not the same green ✓.
func TestModel_FailedRepoMarksDistinctly(t *testing.T) {
	m := New(2)

	nm, _ := m.Update(ProgressMsg{Done: 1, Total: 2, Repo: "repo-ok", State: reconcile.OK, Failed: false})
	m = nm.(Model)
	nm, _ = m.Update(ProgressMsg{Done: 2, Total: 2, Repo: "repo-bad", State: reconcile.Wire, Failed: true})
	m = nm.(Model)

	view := m.View()
	if !strings.Contains(view, "✓") {
		t.Fatalf("view must still mark the successful repo with ✓, got:\n%s", view)
	}
	if !strings.Contains(view, "✗") {
		t.Fatalf("view must mark the failed repo with ✗, got:\n%s", view)
	}
}

func TestBar_FillsProportionally(t *testing.T) {
	// 5 of 10 over width 10 → 5 filled, 5 empty (styles are no-op-able).
	out := Bar(5, 10, 10, lipglossNoop(), lipglossNoop())
	if got := strings.Count(out, "▰"); got != 5 {
		t.Fatalf("filled cells = %d, want 5 (%q)", got, out)
	}
	if got := strings.Count(out, "▱"); got != 5 {
		t.Fatalf("empty cells = %d, want 5 (%q)", got, out)
	}
}

func lipglossNoop() lipgloss.Style { return lipgloss.NewStyle() }
