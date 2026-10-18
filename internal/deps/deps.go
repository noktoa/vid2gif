// Package deps reads deps.json, finds the pinned yt-dlp and ffmpeg, and installs them.
package deps

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type Asset struct {
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Archive string `json:"archive"` // none | zip | tar.xz
}

type Tool struct {
	Version   string           `json:"version"`
	Platforms map[string]Asset `json:"platforms"`
}

type Pins map[string]Tool

func Load(path string) (Pins, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d Pins
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

func platformKey() string { return runtime.GOOS + "-" + runtime.GOARCH }

func (t Tool) AssetFor(key string) (Asset, error) {
	a, ok := t.Platforms[key]
	if !ok {
		return Asset{}, fmt.Errorf("unsupported platform %s; use --no-pin with your own yt-dlp and ffmpeg on PATH", key)
	}
	return a, nil
}

func ExeName(name string) string {
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
// with the pinned build; deno's first line must be "deno <pin> (...".
func versionMatches(name, pin, out string) bool {
	if name == "ffmpeg" {
		return strings.HasPrefix(strings.TrimPrefix(firstLine(out), "ffmpeg version "), pin)
	}
	if name == "deno" { // "deno 2.9.7 (stable, release, ...)"
		return strings.HasPrefix(firstLine(out), "deno "+pin+" ")
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
