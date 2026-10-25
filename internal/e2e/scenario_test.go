package e2e

import (
	"strings"
	"testing"
)

const goodScenario = `# ZEN-9: Create ticket
Vai: nhân viên //znf:allow-lang

## @ac:1 [e2e] Khi tôi tạo ticket thiếu tiêu đề thì thấy lỗi //znf:allow-lang
Bước: //znf:allow-lang
1. Mở form tạo ticket //znf:allow-lang
2. Để trống tiêu đề //znf:allow-lang
3. Bấm Tạo //znf:allow-lang
Thấy: //znf:allow-lang
- Vui lòng nhập tiêu đề //znf:allow-lang
Dữ liệu sau: //znf:allow-lang
- không có ticket mới //znf:allow-lang

## @ac:2 [ngoài-e2e] Email gửi đúng giờ //znf:allow-lang

## Câu hỏi mở //znf:allow-lang
- Có gửi email không? //znf:allow-lang
`

func TestParseScenario(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		// strip the lang-gate markers used only to satisfy the repo language gate
		src := strings.ReplaceAll(goodScenario, " //znf:allow-lang", "")
		sc, err := ParseScenario(src)
		if err != nil {
			t.Fatal(err)
		}
		if sc.Key != "ZEN-9" || sc.Role != "nhân viên" || len(sc.ACs) != 2 { //znf:allow-lang
			t.Fatalf("bad scenario: %+v", sc)
		}
		if len(sc.ACs[0].Steps) != 3 || sc.ACs[0].Sees[0] != "Vui lòng nhập tiêu đề" { //znf:allow-lang
			t.Fatalf("bad AC0: %+v", sc.ACs[0])
		}
		if sc.ACs[0].Kind != ACKindE2E || sc.ACs[1].Kind != ACKindOut || sc.ACs[1].N != 2 {
			t.Fatalf("bad kinds: %+v", sc.ACs)
		}
		if sc.Raw != src {
			t.Fatal("Raw must hold the source verbatim")
		}
	})
	cases := []struct{ name, src, want string }{
		{"unknown kind", "# ZEN-1: t\nVai: a\n\n## @ac:1 [abc] x\n", "loại @ac"}, //znf:allow-lang
		{"duplicate ac", "# ZEN-1: t\nVai: a\n\n## @ac:1 [e2e] x\n\n## @ac:1 [e2e] y\n", "duplicate"},
		{"no ac", "# ZEN-1: t\nVai: a\n", "no @ac"}, //znf:allow-lang
		{"bad title", "hello\n## @ac:1 [e2e] x\n", "ZEN-"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseScenario(c.src)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}
