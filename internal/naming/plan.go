package naming

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

var AllFormats = []string{"gif", "webp", "mp4", "mov"}

func ParseFormats(s string) ([]string, error) {
	var out []string
	for _, f := range strings.Split(s, ",") {
		f = strings.ToLower(strings.TrimSpace(f))
		if !slices.Contains(AllFormats, f) {
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
	// 80 characters, and at most 200 bytes so the time suffix and extension
	// still fit in a 255-byte Linux file name (CJK and emoji take 3-4 bytes each).
	r := []rune(s)
	if len(r) > 80 {
		r = r[:80]
	}
	for len(string(r)) > 200 {
		r = r[:len(r)-1]
	}
	s = string(r)
	s = strings.TrimRight(s, ". ")
	if s == "" {
		return "clip"
	}
	return s
}

// BuildName returns <title>_<start>-<end>; end 0 means "until the end of the
// video" and is written as "end".
func BuildName(title string, start, end float64) string {
	endLabel := "end"
	if end > 0 {
		endLabel = FmtSeconds(end)
	}
	return sanitizeTitle(title) + "_" + FmtSeconds(start) + "-" + endLabel
}

func OutPath(outDir, name, format string) string {
	return filepath.Join(outDir, name+"."+format)
}

// PlanOutputs splits formats into those to write and those skipped because
// the output file already exists (unless overwrite is set).
func PlanOutputs(outDir, name string, formats []string, overwrite bool) (todo, skipped []string) {
	for _, f := range formats {
		if _, err := os.Stat(OutPath(outDir, name, f)); err == nil && !overwrite {
			skipped = append(skipped, f)
		} else {
			todo = append(todo, f)
		}
	}
	return todo, skipped
}
