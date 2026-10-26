package manifest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

// KnownFeatures lists the feature keys the personal overlay's `features:` map accepts.
var KnownFeatures = []string{"github.issues"}

// ReadFeatures returns the known feature toggles set in the overlay. A missing
// file yields an empty map; keys outside KnownFeatures are ignored.
func ReadFeatures(overlayPath string) (map[string]bool, error) {
	out := map[string]bool{}
	b, err := os.ReadFile(overlayPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own workspace state
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read overlay: %w", err)
	}
	var ov struct {
		Features map[string]yaml.Node `yaml:"features"`
	}
	if err := yaml.Unmarshal(b, &ov); err != nil {
		return nil, fmt.Errorf("parse overlay: %w", err)
	}
	for _, k := range KnownFeatures {
		if n, ok := ov.Features[k]; ok {
			var v bool
			if err := n.Decode(&v); err != nil {
				return nil, fmt.Errorf("parse overlay: feature %q: %w", k, err)
			}
			out[k] = v
		}
	}
	return out, nil
}

// SetFeature writes features.<key> into the overlay, preserving every other
// node and comment. The file is created when absent; a broken file is never
// overwritten.
func SetFeature(overlayPath, key string, on bool) error {
	if !slices.Contains(KnownFeatures, key) {
		return fmt.Errorf("unknown feature %q", key)
	}
	var doc yaml.Node
	b, err := os.ReadFile(overlayPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own workspace state
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return fmt.Errorf("read overlay: %w", err)
	default:
		if err := yaml.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("parse overlay: %w", err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("parse overlay: top level is not a mapping")
	}
	feats := mapValue(root, "features")
	if feats == nil {
		feats = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "features"}, feats)
	}
	if feats.Kind == yaml.ScalarNode && feats.Tag == "!!null" {
		*feats = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	if feats.Kind != yaml.MappingNode {
		return fmt.Errorf("parse overlay: features is not a mapping")
	}
	val := fmt.Sprint(on)
	if v := mapValue(feats, key); v != nil {
		v.Kind, v.Tag, v.Value, v.Style = yaml.ScalarNode, "!!bool", val, 0
	} else {
		feats.Content = append(feats.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: val})
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("encode overlay: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("encode overlay: %w", err)
	}
	out := buf.Bytes()
	tmp, err := os.CreateTemp(filepath.Dir(overlayPath), ".overlay-*.tmp")
	if err != nil {
		return fmt.Errorf("write overlay: %w", err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // best-effort cleanup; no-op after a successful rename
	if _, err := tmp.Write(out); err != nil {
		tmp.Close() //nolint:errcheck,gosec // already failing
		return fmt.Errorf("write overlay: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write overlay: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { //nolint:gosec // overlay is a user-readable workspace file
		return fmt.Errorf("write overlay: %w", err)
	}
	if err := os.Rename(tmp.Name(), overlayPath); err != nil {
		return fmt.Errorf("write overlay: %w", err)
	}
	return nil
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}
