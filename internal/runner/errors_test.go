package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func TestToolTrouble(t *testing.T) {
	for _, c := range []struct {
		err  string
		want bool
	}{
		{"ERROR: http error 403", true},
		{"ffmpeg: exit status 1\nsomething odd", true},
		{"ERROR: Private video", false},
		{"dial tcp: no such host", false},
		{"permission denied", false},
	} {
		if got := ToolTrouble(PlainError(errors.New(c.err), false)); got != c.want {
			t.Errorf("ToolTrouble(PlainError(%q)) = %v, want %v", c.err, got, c.want)
		}
	}
	if ToolTrouble("") {
		t.Error(`ToolTrouble("") = true`)
	}
}

// TestPlainErrorRealOfflineOutput feeds yt-dlp's real Windows stderr (pinned
// 2026.08.19, captured 2026-10-19): an unresolvable host, and a dead proxy
// for YouTube (urllib) and Vimeo (curl_cffi).
func TestPlainErrorRealOfflineOutput(t *testing.T) {
	const offline = "Couldn't reach the internet. Check your connection and try again."
	for _, name := range []string{"offline-dns.txt", "offline-proxy-youtube.txt", "offline-proxy-curl.txt"} {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if got := PlainError(fmt.Errorf("yt-dlp: exit status 1\n%s", b), false); got != offline {
			t.Errorf("%s: PlainError = %q, want the internet message", name, got)
		}
	}
	for _, s := range []string{
		"[WinError 10060] A connection attempt failed because the connected party did not properly respond",
		"[WinError 10065] A socket operation was attempted to an unreachable host",
		"Connection refused",
	} {
		if got := PlainError(errors.New("yt-dlp: exit status 1\nERROR: "+s), false); got != offline {
			t.Errorf("%q: PlainError = %q", s, got)
		}
	}
	// A private video seen through a flaky connection is still private.
	if got := PlainError(errors.New("ERROR: Private video. getaddrinfo failed"), false); !strings.HasPrefix(got, "Couldn't get this video") {
		t.Errorf("private lost precedence: %q", got)
	}
}
