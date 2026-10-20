package deps

import (
	"fmt"
	"io/fs"
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

// ResolveOwn is Resolve for the desktop app: a tool in binDir must match its
// pin, but one found only on PATH is the user's own and is used as is. note
// says how it differs from the pin ("" when it doesn't). A tool in neither
// place is an error wrapping fs.ErrNotExist.
func (r Resolver) ResolveOwn(name string, t Tool) (path, note string, err error) {
	if _, serr := os.Stat(filepath.Join(r.binDir, ExeName(name))); serr == nil {
		path, err = r.Resolve(name, t, false)
		return path, "", err
	}
	path, err = r.lookPath(name)
	if err != nil {
		return "", "", fmt.Errorf("%s not found in %s or on PATH: %w", name, r.binDir, fs.ErrNotExist)
	}
	out, err := r.version(path, name)
	switch {
	case err != nil:
		note = fmt.Sprintf("%s: couldn't read its version (%v); using it anyway", path, err)
	case !versionMatches(name, t.Version, out):
		note = fmt.Sprintf("%s is %s, not the pinned %s; using it anyway", path, firstLine(out), t.Version)
	}
	return path, note, nil
}
