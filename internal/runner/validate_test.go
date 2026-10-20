package runner

import "testing"

func TestValidate(t *testing.T) {
	const (
		needStart = "Add a start time to include this one."
		badTime   = "That time doesn't look right. Try 1:23 or 0:01:23."
		order     = "Stop must be after Start."
		startPast = "That's past the end of the video."
		endPast   = "Stop is past the end of the video."
		zeroLen   = "Length must be more than zero."
		lenPast   = "Length runs past the end of the video."
	)
	cases := []struct {
		start, end string
		length     bool // the second box is a length, not a stop time
		dur        float64
		msg        string
		s, e       float64
	}{
		{"", "", false, 0, needStart, 0, 0},
		{"", "1:00", false, 0, needStart, 0, 0},
		{"1:12", "1:19", false, 0, "", 72, 79},
		{"1:12", "", false, 0, "", 72, 0},
		{"abc", "", false, 0, badTime, 0, 0},
		{"1:12", "abc", false, 0, badTime, 0, 0},
		{"1:19", "1:12", false, 0, order, 0, 0},
		{"1:12", "1:12", false, 0, order, 0, 0},
		{"5:00", "", false, 120, startPast, 0, 0},
		{"1:00", "3:00", false, 120, endPast, 0, 0},
		{"1:00", "2:00", false, 120, "", 60, 120},
		{"0:01:23.5", "", false, 120, "", 83.5, 0},
		// unknown length (live): any times
		{"5:00", "", false, 0, "", 300, 0},
		{"1:00", "3:00", false, 0, "", 60, 180},
		// the second box as a length: end is start plus it
		{"1:12", "7", true, 0, "", 72, 79},
		{"1:12", "0:07.5", true, 0, "", 72, 79.5},
		{"1:12", "", true, 0, "", 72, 0},
		{"1:12", "0", true, 0, zeroLen, 0, 0},
		{"1:12", "abc", true, 0, badTime, 0, 0},
		{"", "7", true, 0, needStart, 0, 0},
		{"1:00", "1:00", true, 120, "", 60, 120},
		{"1:00", "1:01", true, 120, lenPast, 0, 0},
		{"5:00", "7", true, 120, startPast, 0, 0},
	}
	for _, c := range cases {
		s, e, msg := Validate(c.start, c.end, c.length, c.dur)
		if msg != c.msg {
			t.Errorf("Validate(%q, %q, %v, %v) msg = %q, want %q", c.start, c.end, c.length, c.dur, msg, c.msg)
			continue
		}
		if msg == "" && (s != c.s || e != c.e) {
			t.Errorf("Validate(%q, %q, %v, %v) = %v, %v, want %v, %v", c.start, c.end, c.length, c.dur, s, e, c.s, c.e)
		}
	}
}
