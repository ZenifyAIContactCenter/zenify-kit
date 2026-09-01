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
	// AWS access key id — pattern gitleaks default bắt được. KHÔNG dùng
	// AKIAIOSFODNN7EXAMPLE: gitleaks default config allowlist mọi secret
	// kết thúc bằng "EXAMPLE" (regex `.+EXAMPLE$`), nên giá trị đó không
	// bao giờ được phát hiện — xác nhận bằng cách đọc config/gitleaks.toml
	// đã cài đặt (rule aws-access-token, allowlists).
	// Split so no contiguous 20-char AKIA... literal sits in source (the
	// repo's own `secret-scan .` CI step would otherwise flag this file);
	// the concatenation still equals the full key at runtime.
	const secret = "AKIA" + "QWERTYUIOPASDFGH"
	fs := s.ScanText("f.txt", "aws_key = "+secret+"\n")
	if len(fs) == 0 {
		t.Fatal("phải phát hiện AWS key")
	}
	for _, f := range fs {
		if f.RuleID == "" {
			t.Error("finding thiếu RuleID")
		}
		// FR-041: không lộ value nguyên.
		if strings.Contains(f.Redacted, secret) {
			t.Error("Redacted lộ secret nguyên")
		}
	}
}

func TestScanTextClean(t *testing.T) {
	s, _ := New()
	if fs := s.ScanText("f.txt", "hello = world\n"); len(fs) != 0 {
		t.Fatalf("clean text không được có finding, có %d", len(fs))
	}
}
