// Package settings loads and saves the desktop app's preferences.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"vid2gif/internal/encode"
)

type Settings struct {
	Theme      string   `json:"theme"`
	OutDir     string   `json:"outDir"`
	Formats    []string `json:"formats"`
	Quality    string   `json:"quality"`
	FullSource bool     `json:"fullSource"` // false = the faster download
	DebugMode  bool     `json:"debugMode"`
}

var Themes = []string{"grey", "mint", "lilac"}

func Default() Settings {
	home, _ := os.UserHomeDir()
	return Settings{
		Theme:   "grey",
		OutDir:  filepath.Join(home, "Videos", "Clips"),
		Formats: []string{"gif"},
		Quality: "medium",
	}
}

// Load never fails: a missing or corrupt file gives the defaults, and each
// invalid field falls back on its own so a bad theme keeps a good folder.
func Load(path string) Settings {
	d := Default()
	b, err := os.ReadFile(path)
	if err != nil {
		return d
	}
	var s Settings
	if json.Unmarshal(b, &s) != nil {
		return d
	}
	if !slices.Contains(Themes, s.Theme) {
		s.Theme = d.Theme
	}
	if !ValidOutDir(s.OutDir, filepath.Dir(path)) {
		s.OutDir = d.OutDir
	}
	if len(s.Formats) == 0 {
		s.Formats = d.Formats
	}
	if _, ok := encode.Presets[s.Quality]; !ok {
		s.Quality = d.Quality
	}
	return s
}

// LocalAbs reports whether p is an absolute path on a local drive: not
// relative or drive-relative (C:x), and not UNC or a device path (\\host,
// //host, \\?\, \\.\), which Windows would reach over the network.
// It only looks at the text, never the disk.
func LocalAbs(p string) bool {
	if strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, `//`) {
		return false
	}
	p = filepath.Clean(p)
	return filepath.IsAbs(p) && !strings.HasPrefix(p, `\\`)
}

// ValidOutDir reports whether dir may be the output folder: a local absolute
// path, not a drive or filesystem root, and not inside dataRoot (the app's
// data folder, which uninstall deletes). Lexical; case-insensitive on Windows.
func ValidOutDir(dir, dataRoot string) bool {
	if strings.TrimSpace(dir) == "" || !LocalAbs(dir) {
		return false
	}
	dir = filepath.Clean(dir)
	if filepath.Dir(dir) == dir {
		return false
	}
	if dataRoot == "" {
		return true
	}
	rel, err := filepath.Rel(filepath.Clean(dataRoot), dir)
	inside := err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	return !inside
}

// Save writes via a temp file and rename so a crash never leaves a half-written file.
func (s Settings) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
