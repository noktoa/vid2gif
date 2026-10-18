package deps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Resolver finds a tool (in binDir first, then PATH) and, unless noPin,
// checks it against the pinned version.
type Resolver struct {
	binDir   string
	lookPath func(string) (string, error)
	version  func(path, name string) (string, error)
}

func NewResolver(binDir string) Resolver {
	return Resolver{binDir: binDir, lookPath: exec.LookPath, version: runVersion}
}

func (r Resolver) Resolve(name string, t Tool, noPin bool) (string, error) {
	path := filepath.Join(r.binDir, ExeName(name))
	if _, err := os.Stat(path); err != nil {
		p, err := r.lookPath(name)
		if err != nil {
			return "", fmt.Errorf("%s not found in %s or on PATH; run `vid2gif deps install`", name, r.binDir)
		}
		path = p
	}
	if noPin {
		return path, nil
	}
	out, err := r.version(path, name)
	if err != nil {
		return "", fmt.Errorf("%s: running version check: %w; run `vid2gif deps install`", path, err)
	}
	if !versionMatches(name, t.Version, out) {
		return "", fmt.Errorf("%s version mismatch: pinned %s, found %s at %s; run `vid2gif deps install`",
			name, t.Version, firstLine(out), path)
	}
	return path, nil
}
