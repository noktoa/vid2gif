package runner

import "testing"

func TestValidate(t *testing.T) {
	const (
		needStart = "Add a start time to include this one."
		badTime   = "That time doesn't look right. Try 1:23 or 0:01:23."
		order     = "Stop must be after Start."
		startPast = "That's past the end of the video."
		endPast   = "Stop is past the end of the video."
	)
	cases := []struct {
		start, end string
		dur        float64
		msg        string
		s, e       float64
	}{
		{"", "", 0, needStart, 0, 0},
		{"", "1:00", 0, needStart, 0, 0},
		{"1:12", "1:19", 0, "", 72, 79},
		{"1:12", "", 0, "", 72, 0},
		{"abc", "", 0, badTime, 0, 0},
		{"1:12", "abc", 0, badTime, 0, 0},
		{"1:19", "1:12", 0, order, 0, 0},
		{"1:12", "1:12", 0, order, 0, 0},
		{"5:00", "", 120, startPast, 0, 0},
		{"1:00", "3:00", 120, endPast, 0, 0},
		{"1:00", "2:00", 120, "", 60, 120},
		{"0:01:23.5", "", 120, "", 83.5, 0},
		// unknown length (live): any times
		{"5:00", "", 0, "", 300, 0},
		{"1:00", "3:00", 0, "", 60, 180},
	}
	for _, c := range cases {
		s, e, msg := Validate(c.start, c.end, c.dur)
		if msg != c.msg {
			t.Errorf("Validate(%q, %q, %v) msg = %q, want %q", c.start, c.end, c.dur, msg, c.msg)
			continue
		}
		if msg == "" && (s != c.s || e != c.e) {
			t.Errorf("Validate(%q, %q, %v) = %v, %v, want %v, %v", c.start, c.end, c.dur, s, e, c.s, c.e)
		}
	}
}
