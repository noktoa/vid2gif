// Package vid2gif only carries the pinned tool list into the binaries.
package vid2gif

import _ "embed"

// DepsJSON is deps.json, so a desktop build needs no file next to the exe.
//
//go:embed deps.json
var DepsJSON []byte
