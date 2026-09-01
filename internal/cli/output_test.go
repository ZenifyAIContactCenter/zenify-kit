package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, map[string]bool{"healthy": true}); err != nil {
		t.Fatal(err)
	}
	var got Envelope
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SchemaVersion != "1" {
		t.Errorf("SchemaVersion = %q", got.SchemaVersion)
	}
	data := got.Data.(map[string]any)
	if data["healthy"] != true {
		t.Errorf("Data.healthy = %v", data["healthy"])
	}
}
