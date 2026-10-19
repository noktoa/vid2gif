package runner

import (
	"errors"
	"strings"
	"testing"
)

func TestPlainError(t *testing.T) {
	const (
		gone    = "Couldn't get this video. It may be private or removed."
		offline = "Couldn't reach the internet. Check your connection and try again."
		folder  = "Couldn't save to that folder. Pick another one."
		site    = "This site isn't working with the app right now. An update may fix it."
		generic = "Something went wrong with this one."
		hintOff = "Turn on troubleshooting in Settings and try again to save details."
		hintOn  = "Details were saved to the log."
	)
	cases := []struct {
		err   string
		debug bool
		want  string
	}{
		{"yt-dlp: exit status 1\nERROR: [youtube] abc: Private video. Sign in if you've been granted access", false, gone},
		{"yt-dlp: exit status 1\nERROR: [youtube] abc: Video unavailable", false, gone},
		{"ERROR: Video unavailable. Sign in to confirm your age", false, gone}, // private/removed wins
		{"dial tcp: lookup www.youtube.com: no such host", false, offline},
		{"yt-dlp: exit status 1\nERROR: dial tcp 1.2.3.4:443: connect refused", false, offline},
		{"Read timeout", false, offline},
		{"open C:\\clips\\.vid2gif-tmp-1: Access is denied.", false, folder},
		{"mkdir /clips/.vid2gif-tmp-1: permission denied", false, folder},
		{"rename x y: no such file or directory", false, folder},
		{"ERROR: [youtube] abc: Unable to extract initial player response", false, site},
		{"ERROR: [youtube] abc: Sign in to confirm you're not a bot", false, site},
		{"ERROR: unable to download video data: HTTP Error 403: Forbidden", false, site},
		{"WARNING: nsig extraction failed: You may experience throttling", false, site},
		{"ffmpeg: exit status 1\nsomething odd", false, generic + " " + hintOff},
		{"ffmpeg: exit status 1\nsomething odd", true, generic + " " + hintOn},
	}
	for _, c := range cases {
		got := PlainError(errors.New(c.err), c.debug)
		if got != c.want {
			t.Errorf("PlainError(%q, %v) = %q, want %q", c.err, c.debug, got, c.want)
		}
		for _, bad := range []string{"yt-dlp", "ffmpeg", "exit status"} {
			if strings.Contains(got, bad) {
				t.Errorf("PlainError(%q) leaks %q: %q", c.err, bad, got)
			}
		}
	}
}
