// Package kit exposes build-time assets that live at the module root and
// therefore cannot be embedded from any internal/ package (go:embed only
// reaches files at or below the embedding package's directory).
package kit

import _ "embed"

// DefaultRepos is manifest/repos.yaml compiled into the binary so `zenify up`
// and `zenify down` work from any cwd, not only the kit checkout (W0 FR-05).
//
//go:embed manifest/repos.yaml
var DefaultRepos []byte
