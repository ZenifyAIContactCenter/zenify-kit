package plugin

import (
	"strings"
	"testing"
)

func TestBootstrapIsDigestNotGlobalMandate(t *testing.T) {
	b, err := assets.ReadFile("assets/znf/skills/using-zenify-kit/BOOTSTRAP.txt")
	if err != nil {
		t.Fatalf("BOOTSTRAP.txt not embedded: %v", err)
	}
	s := string(b)
	if n := strings.Count(s, "\n"); n > 45 {
		t.Errorf("digest too long: %d lines (want <=45)", n)
	}
	// Drop the old global-mandate imperative from A (FR-M2E-04).
	if strings.Contains(s, "BẤT KỲ") || strings.Contains(strings.ToLower(s), "before any") { //znf:allow-lang -- literal being tested for absence, not translatable
		t.Errorf("BOOTSTRAP still carries a global 'use znf before any response' mandate")
	}
	for _, anchor := range []string{"blast radius", "No fabrication", "znf:discipline"} {
		if !strings.Contains(s, anchor) {
			t.Errorf("digest missing anchor %q", anchor)
		}
	}
}
