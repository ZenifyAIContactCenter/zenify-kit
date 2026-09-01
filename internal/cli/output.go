package cli

import (
	"encoding/json"
	"io"
)

const envelopeSchemaVersion = "1"

// Envelope is the stable JSON output shape (FR-051): a schema version plus a
// command-specific data payload.
type Envelope struct {
	SchemaVersion string `json:"schema_version"`
	Data          any    `json:"data"`
}

// writeJSON marshals data inside an Envelope with a trailing newline.
func writeJSON(w io.Writer, data any) error {
	b, err := json.MarshalIndent(Envelope{SchemaVersion: envelopeSchemaVersion, Data: data}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}
