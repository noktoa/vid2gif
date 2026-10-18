package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

var allFormats = []string{"gif", "webp", "mp4", "mov"}

func parseFormats(s string) ([]string, error) {
	var out []string
	for _, f := range strings.Split(s, ",") {
		f = strings.ToLower(strings.TrimSpace(f))
		if !slices.Contains(allFormats, f) {
			return nil, fmt.Errorf("unknown format %q (choose from gif, webp, mp4, mov)", f)
		}
		if !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out, nil
}

var invalidFileChars = strings.NewReplacer(`<`, "_", `>`, "_", `:`, "_", `"`, "_", `/`, "_", `\`, "_", `|`, "_", `?`, "_", `*`, "_")

// sanitizeTitle makes a video title safe as a Windows/Linux file name.
func sanitizeTitle(s string) string {
	s = invalidFileChars.Replace(strings.TrimSpace(s))
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '_'
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), "_")
	if r := []rune(s); len(r) > 80 {
		s = string(r[:80])
	}
	s = strings.TrimRight(s, ". ")
	if s == "" {
		return "clip"
	}
	return s
}

func buildName(title string, start, end float64) string {
	return sanitizeTitle(title) + "_" + fmtSeconds(start) + "-" + fmtSeconds(end)
}

func outPath(outDir, name, format string) string {
	return filepath.Join(outDir, name+"."+format)
}

// planOutputs splits formats into those to write and those skipped because
// the output file already exists (unless overwrite is set).
func planOutputs(outDir, name string, formats []string, overwrite bool) (todo, skipped []string) {
	for _, f := range formats {
		if _, err := os.Stat(outPath(outDir, name, f)); err == nil && !overwrite {
			skipped = append(skipped, f)
		} else {
			todo = append(todo, f)
		}
	}
	return todo, skipped
}
