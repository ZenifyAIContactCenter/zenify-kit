package gitx

import (
	"strings"
	"testing"
)

// scriptRunner answers by matching the first two git args.
type scriptRunner struct{ resp map[string]string }

func (s scriptRunner) Run(dir string, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	for k, v := range s.resp {
		if strings.HasPrefix(key, k) {
			return []byte(v), nil
		}
	}
	return nil, nil
}

func TestNormalizeRemote(t *testing.T) {
	cases := []struct {
		url       string
		insteadOf map[string]string
		want      string
	}{
		{"git@github.com:ZenifyAIContactCenter/contact-center-be.git", nil, "ZenifyAIContactCenter/contact-center-be"},
		{"git@github-zenify:ZenifyAIContactCenter/chatting.git", map[string]string{"git@github-zenify:": "git@github.com:"}, "ZenifyAIContactCenter/chatting"},
		{"https://github.com/ZenifyAIContactCenter/notification.git", nil, "ZenifyAIContactCenter/notification"},
		{"git@github-zenify:ZenifyAIContactCenter/x.git", nil, "ZenifyAIContactCenter/x"}, // alias host stripped even without insteadOf
	}
	for _, c := range cases {
		if got := NormalizeRemote(c.url, c.insteadOf); got != c.want {
			t.Errorf("NormalizeRemote(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestScanNotCloned(t *testing.T) {
	st, err := Scan(scriptRunner{}, t.TempDir())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if st.Cloned {
		t.Errorf("empty dir should be not-cloned")
	}
}
