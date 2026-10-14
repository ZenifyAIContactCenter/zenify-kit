package devenv

import (
	"bytes"
	"testing"
)

func maskRun(t *testing.T, secrets []string, chunks ...string) string {
	t.Helper()
	var buf bytes.Buffer
	m := NewMaskWriter(&buf, secrets)
	for _, c := range chunks {
		if n, err := m.Write([]byte(c)); err != nil || n != len(c) {
			t.Fatalf("Write = %d,%v", n, err)
		}
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestMask(t *testing.T) {
	if got := maskRun(t, []string{"abc#def12"}, "x abc#def12 y\nabc#def12"); got != "x *** y\n***" {
		t.Errorf("got %q", got)
	}
}

func TestMask_SplitAcrossWrites(t *testing.T) {
	if got := maskRun(t, []string{"abc#def12"}, "x abc#d", "ef12 y\n"); got != "x *** y\n" {
		t.Errorf("got %q", got)
	}
}

func TestMask_ShortSecretIgnored(t *testing.T) {
	if got := maskRun(t, []string{"short"}, "a short b\n"); got != "a short b\n" {
		t.Errorf("got %q", got)
	}
}

func TestMask_TwoSecretsOneLine(t *testing.T) {
	if got := maskRun(t, []string{"secret-one", "secret-two"}, "secret-one and secret-two\n"); got != "*** and ***\n" {
		t.Errorf("got %q", got)
	}
}

func TestMask_LongerFirst(t *testing.T) {
	if got := maskRun(t, []string{"abcdef", "abcdef-tail"}, "abcdef-tail\n"); got != "***\n" {
		t.Errorf("got %q", got)
	}
}
