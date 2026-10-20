package runner

import (
	"context"
	"errors"
	"strings"
)

const (
	folderMsg  = "Couldn't save to that folder. Pick another one."
	siteMsg    = "This site isn't working with the app right now. An update may fix it."
	genericMsg = "Something went wrong with this one."
)

// ToolTrouble reports whether a PlainError message is one an out-of-date
// helper tool could cause: the site-broken and don't-know messages, not a
// private video, no internet or an unwritable folder.
func ToolTrouble(msg string) bool {
	return strings.HasPrefix(msg, siteMsg) || strings.HasPrefix(msg, genericMsg)
}

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
		siteMsg},
	// Go's net errors, then yt-dlp's (Python) on Windows: see testdata/offline-*.txt.
	{[]string{"no such host", "dial tcp", "timeout", "timed out", "network is unreachable",
		"getaddrinfo failed", "failed to resolve", "unable to connect to proxy", "failed to establish a new connection",
		"could not connect to server", "connection refused", "winerror 10060", "winerror 10061", "winerror 10065"},
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
		return genericMsg + " Details were saved to the log."
	}
	return genericMsg + " Turn on troubleshooting in Settings and try again to save details."
}
