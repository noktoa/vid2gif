package naming

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseTimestamp parses SS, MM:SS or HH:MM:SS into seconds. Only the last
// component may have a fractional part; minutes and seconds after the first
// component must be below 60.
func ParseTimestamp(s string) (float64, error) {
	bad := fmt.Errorf("invalid timestamp %q (use SS, MM:SS or HH:MM:SS)", s)
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) > 3 {
		return 0, bad
	}
	total := 0.0
	for i, p := range parts {
		if p == "" || strings.Trim(p, "0123456789.") != "" {
			return 0, bad
		}
		var v float64
		var err error
		if i == len(parts)-1 {
			v, err = strconv.ParseFloat(p, 64)
		} else {
			var n int
			n, err = strconv.Atoi(p)
			v = float64(n)
		}
		if err != nil || (i > 0 && v >= 60) {
			return 0, bad
		}
		total = total*60 + v
	}
	return total, nil
}

// FmtSeconds renders seconds compactly (millisecond precision) for file names
// and yt-dlp section arguments.
func FmtSeconds(sec float64) string {
	return strconv.FormatFloat(math.Round(sec*1000)/1000, 'f', -1, 64)
}
