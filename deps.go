package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type asset struct {
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Archive string `json:"archive"` // none | zip | tar.xz
}

type tool struct {
	Version   string           `json:"version"`
	Platforms map[string]asset `json:"platforms"`
}

type deps map[string]tool

func loadDeps(path string) (deps, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d deps
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

func platformKey() string { return runtime.GOOS + "-" + runtime.GOARCH }

func (t tool) assetFor(key string) (asset, error) {
	a, ok := t.Platforms[key]
	if !ok {
		return asset{}, fmt.Errorf("unsupported platform %s; use --no-pin with your own yt-dlp and ffmpeg on PATH", key)
	}
	return a, nil
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}

// versionMatches compares a tool's version output with its pin. yt-dlp must
// match exactly; ffmpeg's "ffmpeg version <build>-<date> ..." line must start
// with the pinned build.
func versionMatches(name, pin, out string) bool {
	if name == "ffmpeg" {
		return strings.HasPrefix(strings.TrimPrefix(firstLine(out), "ffmpeg version "), pin)
	}
	return strings.TrimSpace(out) == pin
}

func versionFlag(name string) string {
	if name == "ffmpeg" {
		return "-version"
	}
	return "--version"
}

func runVersion(path, name string) (string, error) {
	out, err := exec.Command(path, versionFlag(name)).Output()
	return string(out), err
}

// resolver finds a tool (bin/ next to deps.json first, then PATH) and, unless
// noPin, checks it against the pinned version.
type resolver struct {
	baseDir  string
	lookPath func(string) (string, error)
	version  func(path, name string) (string, error)
}

func newResolver(baseDir string) resolver {
	return resolver{baseDir: baseDir, lookPath: exec.LookPath, version: runVersion}
}

func (r resolver) resolve(name string, t tool, noPin bool) (string, error) {
	binDir := filepath.Join(r.baseDir, "bin")
	path := filepath.Join(binDir, exeName(name))
	if _, err := os.Stat(path); err != nil {
		p, err := r.lookPath(name)
		if err != nil {
			return "", fmt.Errorf("%s not found in %s or on PATH; run `vid2gif deps install`", name, binDir)
		}
		path = p
	}
	if noPin {
		return path, nil
	}
	out, err := r.version(path, name)
	if err != nil {
		return "", fmt.Errorf("%s: running version check: %w", path, err)
	}
	if !versionMatches(name, t.Version, out) {
		return "", fmt.Errorf("%s version mismatch: pinned %s, found %s at %s; run `vid2gif deps install`",
			name, t.Version, firstLine(out), path)
	}
	return path, nil
}
