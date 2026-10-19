// Package lookup turns pasted links into a list of videos with titles,
// durations and thumbnails, using one yt-dlp call per link.
package lookup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"vid2gif/internal/clip"
	"vid2gif/internal/proc"
	"vid2gif/internal/debuglog"
)

const pastedSource = "Pasted links"

// Video is one lookup result. Unavailable entries (private or removed
// playlist items) carry a short Reason and have no usable duration.
type Video struct {
	ID, URL, Title, ThumbnailURL string
	DurationSec                  float64
	Unavailable                  bool
	Reason                       string
}

// ParseInput splits pasted text on whitespace and commas and keeps the
// distinct http(s) links, first occurrence first.
func ParseInput(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' }) {
		u, err := url.Parse(f)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// entry is the part of yt-dlp's --dump-json line we read. A playlist entry
// has `thumbnails` and no `thumbnail`; a single video has the reverse.
// For a single video `url` is a signed media URL, so webpage_url wins.
type entry struct {
	ID            string                 `json:"id"`
	Title         string                 `json:"title"`
	Duration      *float64               `json:"duration"`
	Thumbnail     string                 `json:"thumbnail"`
	Thumbnails    []struct{ URL string } `json:"thumbnails"`
	WebpageURL    string                 `json:"webpage_url"`
	URL           string                 `json:"url"`
	PlaylistTitle string                 `json:"playlist_title"`
}

// unavailable maps yt-dlp's placeholder titles to a Reason.
var unavailable = map[string]string{
	"[Private video]": "private",
	"[Deleted video]": "removed",
}

// Parse reads yt-dlp JSON lines: one for a single video, one per entry for a
// playlist. source is the playlist title, or "Pasted links" without one.
func Parse(jsonl []byte) (source string, vids []Video, err error) {
	source = pastedSource
	for _, line := range bytes.Split(jsonl, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e entry
		if err := json.Unmarshal(line, &e); err != nil {
			return "", nil, fmt.Errorf("reading yt-dlp output: %w", err)
		}
		if e.PlaylistTitle != "" && source == pastedSource {
			source = e.PlaylistTitle
		}
		v := Video{ID: e.ID, URL: e.WebpageURL, Title: e.Title, ThumbnailURL: e.Thumbnail}
		if v.URL == "" {
			v.URL = e.URL
		}
		if v.ThumbnailURL == "" && len(e.Thumbnails) > 0 {
			v.ThumbnailURL = e.Thumbnails[len(e.Thumbnails)-1].URL // largest last
		}
		if e.Duration != nil {
			v.DurationSec = *e.Duration
		}
		if reason, ok := unavailable[e.Title]; ok {
			v.Unavailable, v.Reason = true, reason
		}
		vids = append(vids, v)
	}
	if len(vids) == 0 {
		return "", nil, fmt.Errorf("yt-dlp found no videos at that link")
	}
	return source, vids, nil
}

// Looker runs the pinned yt-dlp. Log may be nil.
type Looker struct {
	YtDlp, Deno string
	Log         *debuglog.Log
}

// lookupArgs: --flat-playlist keeps a playlist to one cheap line per entry,
// and --no-playlist makes a watch?v=..&list=.. link mean just that video.
func lookupArgs(link, deno string) []string {
	args := append([]string{"--ignore-config"}, clip.JSRuntimeArgs(deno)...)
	return append(args, "--flat-playlist", "--dump-json", "--no-playlist", "--", link)
}

// Lookup returns the videos behind one link. On failure the error text keeps
// yt-dlp's stderr so runner.PlainError can classify it.
func (l Looker) Lookup(ctx context.Context, link string) (source string, vids []Video, err error) {
	args := lookupArgs(link, l.Deno)
	l.Log.Printf("lookup: %s %s", l.YtDlp, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, l.YtDlp, args...)
	proc.Hide(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		if line != "" {
			l.Log.Printf("yt-dlp: %s", line)
		}
	}
	if runErr != nil {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		return "", nil, fmt.Errorf("yt-dlp: %w\n%s", runErr, strings.TrimSpace(stderr.String()))
	}
	return Parse(stdout.Bytes())
}
