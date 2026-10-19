package runner

import (
	"context"
	"errors"
	"strings"
)

const folderMsg = "Couldn't save to that folder. Pick another one."

// plainErrors are checked in order on the lowercased error text, which holds
// yt-dlp's stderr (via clip): private/removed wins over the site-broken
// phrases because yt-dlp often adds "sign in" to an unavailable video.
var plainErrors = []struct {
	phrases []string
	msg     string
}{
	{[]string{"private video", "video unavailable", "has been removed"},
		"Couldn't get this video. It may be private or removed."},
	{[]string{"unable to extract", "sign in to confirm", "http error 403", "nsig extraction failed"},
		"This site isn't working with the app right now. An update may fix it."},
	{[]string{"no such host", "dial tcp", "timeout", "timed out", "network is unreachable"},
		"Couldn't reach the internet. Check your connection and try again."},
	{[]string{"permission denied", "access is denied", "no such file or directory", "not a directory"},
		folderMsg},
}

// PlainError turns a lookup or clip error into a sentence for the slip; it
// never contains tool names or exit codes. A cancel is not an error: "".
func PlainError(err error, debugOn bool) string {
	if err == nil || errors.Is(err, context.Canceled) {
		return ""
	}
	text := strings.ToLower(err.Error())
	for _, p := range plainErrors {
		for _, ph := range p.phrases {
			if strings.Contains(text, ph) {
				return p.msg
			}
		}
	}
	if debugOn {
		return "Something went wrong with this one. Details were saved to the log."
	}
	return "Something went wrong with this one. Turn on troubleshooting in Settings and try again to save details."
}
