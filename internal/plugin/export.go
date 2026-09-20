package plugin

import "io/fs"

// ZnfFS exposes the embedded znf plugin tree (skills/, agents/, …) read-only
// for consumers such as docsgen.
func ZnfFS() fs.FS { sub, _ := fs.Sub(assets, embedRoot); return sub }
