package secretscan

import (
	"strings"
	"testing"
)

func TestScanTextFindsSecret(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// AWS access key id — a pattern gitleaks' default config catches. Do NOT use
	// AKIAIOSFODNN7EXAMPLE: gitleaks' default config allowlists any secret
	// ending in "EXAMPLE" (regex `.+EXAMPLE$`), so that value would
	// never be detected — confirmed by reading the installed config/gitleaks.toml
	// (rule aws-access-token, allowlists).
	// Split so no contiguous 20-char AKIA... literal sits in source (the
	// repo's own `secret-scan .` CI step would otherwise flag this file);
	// the concatenation still equals the full key at runtime.
	const secret = "AKIA" + "QWERTYUIOPASDFGH"
	fs := s.ScanText("f.txt", "aws_key = "+secret+"\n")
	if len(fs) == 0 {
		t.Fatal("must detect the AWS key")
	}
	for _, f := range fs {
		if f.RuleID == "" {
			t.Error("finding missing RuleID")
		}
		if f.Redacted == "" {
			t.Error("Redacted must not be empty")
		}
		// FR-041: must not leak the raw value, even partially (e.g. the first 4 chars).
		if strings.Contains(f.Redacted, secret) {
			t.Error("Redacted leaks the full secret")
		}
		if strings.Contains(f.Redacted, secret[:4]) {
			t.Error("Redacted leaks part of the secret (prefix)")
		}
	}
}

func TestScanTextClean(t *testing.T) {
	s, _ := New()
	if fs := s.ScanText("f.txt", "hello = world\n"); len(fs) != 0 {
		t.Fatalf("clean text must not have findings, got %d", len(fs))
	}
}
