package runner

import (
	"strings"

	"vid2gif/internal/naming"
)

// Validate parses a slip's Start and Stop boxes. An empty msg means valid;
// end 0 means "to the end of the video". durationSec 0 (unknown, e.g. a live
// video) skips the past-the-end checks.
func Validate(startText, endText string, durationSec float64) (start, end float64, msg string) {
	const bad = "That time doesn't look right. Try 1:23 or 0:01:23."
	if strings.TrimSpace(startText) == "" {
		return 0, 0, "Add a start time to include this one."
	}
	start, err := naming.ParseTimestamp(startText)
	if err != nil {
		return 0, 0, bad
	}
	hasEnd := strings.TrimSpace(endText) != ""
	if hasEnd {
		if end, err = naming.ParseTimestamp(endText); err != nil {
			return 0, 0, bad
		}
		if end <= start {
			return 0, 0, "Stop must be after Start."
		}
	}
	if durationSec > 0 {
		if start >= durationSec {
			return 0, 0, "That's past the end of the video."
		}
		if end > durationSec {
			return 0, 0, "Stop is past the end of the video."
		}
	}
	return start, end, ""
}
