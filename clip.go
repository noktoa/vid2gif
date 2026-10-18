package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type clipOpts struct {
	URL        string
	Start, End float64
	Formats    []string
	OutDir     string
	Name       string // empty = derive from the video title
	Quality    string
	Overwrite  bool
	NoPin      bool
}

// pipeline holds the side-effecting steps as fields so run() is testable
// with fakes; newPipeline wires in the real yt-dlp and ffmpeg.
type pipeline struct {
	title    func(ctx context.Context, url string) (string, error)
	download func(ctx context.Context, o clipOpts, tmpDir string, wantAudio bool) (string, error)
	encode   func(ctx context.Context, in, out string, eo encodeOpts) error
	out      io.Writer
}

func (p pipeline) run(ctx context.Context, o clipOpts) error {
	name := o.Name
	if name == "" {
		title, err := p.title(ctx, o.URL)
		if err != nil {
			return fmt.Errorf("fetching title: %w", err)
		}
		name = buildName(title, o.Start, o.End)
	}
	todo, skipped := planOutputs(o.OutDir, name, o.Formats, o.Overwrite)
	for _, f := range skipped {
		fmt.Fprintf(p.out, "skipped %s (already exists; use --overwrite to replace)\n", outPath(o.OutDir, name, f))
	}
	if len(todo) == 0 {
		fmt.Fprintf(p.out, "wrote 0, skipped %d\n", len(skipped))
		return nil
	}
	if err := os.MkdirAll(o.OutDir, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(o.OutDir, ".vid2gif-tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	wantAudio := slices.ContainsFunc(todo, func(f string) bool { return f == "mp4" || f == "mov" })
	clip, err := p.download(ctx, o, tmp, wantAudio)
	if err != nil {
		return err
	}

	wrote, failed := 0, 0
	for _, f := range todo {
		if err := ctx.Err(); err != nil {
			return err
		}
		final := outPath(o.OutDir, name, f)
		part := filepath.Join(tmp, name+"."+f)
		if err := p.encode(ctx, clip, part, encodeOpts{f, presets[o.Quality]}); err != nil {
			fmt.Fprintf(p.out, "failed %s: %v\n", final, err)
			failed++
			continue
		}
		if err := os.Rename(part, final); err != nil {
			fmt.Fprintf(p.out, "failed %s: %v\n", final, err)
			failed++
			continue
		}
		size := int64(0)
		if st, err := os.Stat(final); err == nil {
			size = st.Size()
		}
		fmt.Fprintf(p.out, "wrote %s (%d KB)\n", final, size/1024)
		wrote++
	}
	fmt.Fprintf(p.out, "wrote %d, skipped %d\n", wrote, len(skipped))
	if failed > 0 {
		return fmt.Errorf("%d of %d outputs failed", failed, len(todo))
	}
	return nil
}

// ytdlpTitleArgs asks yt-dlp for the video title only (no download).
// --ignore-config keeps a user's own yt-dlp config from changing a pinned run.
func ytdlpTitleArgs(url string) []string {
	return []string{"--ignore-config", "--print", "title", "--no-playlist", "--", url}
}

// ytdlpDownloadArgs downloads just [Start, End]. --ffmpeg-location is
// mandatory so yt-dlp uses the pinned ffmpeg, and the URL follows `--` so a
// value starting with "-" is never read as an option.
func ytdlpDownloadArgs(o clipOpts, ffmpegPath, tmpDir string, wantAudio bool) []string {
	format := "bv*/b"
	if wantAudio {
		format = "bv*+ba/b"
	}
	return []string{
		"--ignore-config", "--no-playlist",
		"-f", format,
		"--download-sections", fmt.Sprintf("*%s-%s", fmtSeconds(o.Start), fmtSeconds(o.End)),
		"--force-keyframes-at-cuts",
		"--merge-output-format", "mkv",
		"--ffmpeg-location", ffmpegPath,
		"-o", filepath.Join(tmpDir, "clip.%(ext)s"),
		"--", o.URL,
	}
}

// pickClipFile finds yt-dlp's output (clip.<ext>), ignoring partial files.
func pickClipFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, "clip.") && !strings.HasSuffix(n, ".part") && !strings.HasSuffix(n, ".ytdl") {
			return filepath.Join(dir, n), nil
		}
	}
	return "", fmt.Errorf("yt-dlp produced no clip file in %s", dir)
}

func withHint(err error, hint string) error {
	if hint == "" {
		return err
	}
	return fmt.Errorf("%w\n%s", err, hint)
}

func ffmpegEncode(ffmpeg string) func(ctx context.Context, in, out string, eo encodeOpts) error {
	return func(ctx context.Context, in, out string, eo encodeOpts) error {
		b, err := exec.CommandContext(ctx, ffmpeg, ffmpegArgs(in, out, eo)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("ffmpeg: %w\n%s", err, strings.TrimSpace(string(b)))
		}
		return nil
	}
}

// newPipeline wires in the real tools. hint is appended to yt-dlp failures
// (e.g. how to bump the pin).
func newPipeline(ytdlp, ffmpeg, hint string, out io.Writer) pipeline {
	return pipeline{
		title: func(ctx context.Context, url string) (string, error) {
			cmd := exec.CommandContext(ctx, ytdlp, ytdlpTitleArgs(url)...)
			cmd.Stderr = os.Stderr
			b, err := cmd.Output()
			if err != nil {
				return "", withHint(fmt.Errorf("yt-dlp: %w", err), hint)
			}
			return firstLine(string(b)), nil
		},
		download: func(ctx context.Context, o clipOpts, tmpDir string, wantAudio bool) (string, error) {
			cmd := exec.CommandContext(ctx, ytdlp, ytdlpDownloadArgs(o, ffmpeg, tmpDir, wantAudio)...)
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if err := cmd.Run(); err != nil {
				return "", withHint(fmt.Errorf("yt-dlp: %w", err), hint)
			}
			return pickClipFile(tmpDir)
		},
		encode: ffmpegEncode(ffmpeg),
		out:    out,
	}
}
