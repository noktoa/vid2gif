package naming

import "testing"

func TestParseTimestamp(t *testing.T) {
	ok := []struct {
		in   string
		want float64
	}{
		{"0", 0}, {"5", 5}, {"90", 90}, {" 7 ", 7}, {"0.5", 0.5},
		{"1:23", 83}, {"01:02:03", 3723}, {"1:23.5", 83.5},
	}
	for _, c := range ok {
		got, err := ParseTimestamp(c.in)
		if err != nil || got != c.want {
			t.Errorf("ParseTimestamp(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	bad := []string{"", "abc", "-5", "1:2:3:4", "1:75", "1:90:00", "1.5:30", ":30", "1:", "1e3", "NaN", "Inf", "1:23.5.1"}
	for _, in := range bad {
		if got, err := ParseTimestamp(in); err == nil {
			t.Errorf("ParseTimestamp(%q) = %v; want error", in, got)
		}
	}
}

func TestFmtSeconds(t *testing.T) {
	cases := map[float64]string{0: "0", 83: "83", 83.5: "83.5"}
	for in, want := range cases {
		if got := FmtSeconds(in); got != want {
			t.Errorf("FmtSeconds(%v) = %q; want %q", in, got, want)
		}
	}
	sec, _ := ParseTimestamp("1:23.1")
	if got := FmtSeconds(sec); got != "83.1" {
		t.Errorf("FmtSeconds(parse(1:23.1)) = %q; want 83.1", got)
	}
}
