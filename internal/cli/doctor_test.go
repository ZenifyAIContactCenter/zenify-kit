package cli

import (
	"bytes"
	"testing"
)

func TestDoctor_ReportsVersion(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("version")) {
		t.Errorf("doctor output missing 'version': %q", out.String())
	}
}
