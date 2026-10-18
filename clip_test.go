package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type fakeState struct {
	downloads int
	wantAudio bool
	encoded   []string
}

func fakePipeline(out io.Writer) (pipeline, *fakeState) {
	st := &fakeState{}
	return pipeline{
		title: func(ctx context.Context, url string) (string, error) { return "My: Video", nil },
		download: func(ctx context.Context, o clipOpts, tmpDir string, wantAudio bool) (string, error) {
			st.downloads++
			st.wantAudio = wantAudio
			p := filepath.Join(tmpDir, "clip.mkv")
			return p, os.WriteFile(p, []byte("src"), 0o644)
		},
		encode: func(ctx context.Context, in, out string, eo encodeOpts) error {
			st.encoded = append(st.encoded, eo.format)
			return os.WriteFile(out, []byte("data-"+eo.format), 0o644)
		},
		out: out,
	}, st
}

func baseOpts(dir string, formats ...string) clipOpts {
	return clipOpts{URL: "https://example.com/v", Start: 83, End: 88, Formats: formats, OutDir: dir, Quality: "medium"}
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestRunWritesAllFormats(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	p, st := fakePipeline(&out)
	if err := p.run(context.Background(), baseOpts(dir, "gif", "mp4")); err != nil {
		t.Fatal(err)
	}
	want := []string{"My__Video_83-88.gif", "My__Video_83-88.mp4"}
	if got := dirNames(t, dir); !slices.Equal(got, want) {
		t.Errorf("files = %v; want %v (no temp dir may remain)", got, want)
	}
	if st.downloads != 1 || !st.wantAudio {
		t.Errorf("downloads=%d wantAudio=%v", st.downloads, st.wantAudio)
	}
	if !strings.Contains(out.String(), "wrote 2, skipped 0") {
		t.Errorf("summary missing: %q", out.String())
	}
}

func TestRunGifOnlyDownloadsNoAudio(t *testing.T) {
	p, st := fakePipeline(io.Discard)
	if err := p.run(context.Background(), baseOpts(t.TempDir(), "gif", "webp")); err != nil {
		t.Fatal(err)
	}
	if st.wantAudio {
		t.Error("gif/webp only must not request audio")
	}
}

func TestRunSkipsExistingAndWritesRest(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "demo.gif"), []byte("sentinel"), 0o644)
	var out bytes.Buffer
	p, _ := fakePipeline(&out)
	o := baseOpts(dir, "gif", "mp4")
	o.Name = "demo"
	if err := p.run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "demo.gif")); string(b) != "sentinel" {
		t.Errorf("existing gif was modified: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "demo.mp4")); string(b) != "data-mp4" {
		t.Errorf("mp4 not written: %q", b)
	}
	if !strings.Contains(out.String(), "skipped") || !strings.Contains(out.String(), "wrote 1, skipped 1") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunAllSkippedDoesNotDownload(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "demo.gif"), []byte("sentinel"), 0o644)
	p, st := fakePipeline(io.Discard)
	o := baseOpts(dir, "gif")
	o.Name = "demo"
	if err := p.run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if st.downloads != 0 {
		t.Error("must not download when every output already exists")
	}
	if got := dirNames(t, dir); !slices.Equal(got, []string{"demo.gif"}) {
		t.Errorf("files = %v", got)
	}
}

func TestRunOverwriteReplaces(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "demo.gif"), []byte("sentinel"), 0o644)
	p, _ := fakePipeline(io.Discard)
	o := baseOpts(dir, "gif")
	o.Name, o.Overwrite = "demo", true
	if err := p.run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "demo.gif")); string(b) != "data-gif" {
		t.Errorf("gif not replaced: %q", b)
	}
}

func TestRunNameGivenSkipsTitleLookup(t *testing.T) {
	p, _ := fakePipeline(io.Discard)
	p.title = func(context.Context, string) (string, error) {
		t.Fatal("title must not be fetched when --name is set")
		return "", nil
	}
	o := baseOpts(t.TempDir(), "gif")
	o.Name = "demo"
	if err := p.run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
}

func TestRunEncodeFailureContinuesAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	p, _ := fakePipeline(io.Discard)
	realEncode := p.encode
	p.encode = func(ctx context.Context, in, out string, eo encodeOpts) error {
		if eo.format == "gif" {
			return errors.New("boom")
		}
		return realEncode(ctx, in, out, eo)
	}
	o := baseOpts(dir, "gif", "mp4")
	o.Name = "demo"
	err := p.run(context.Background(), o)
	if err == nil {
		t.Fatal("expected an error when one format fails")
	}
	if got := dirNames(t, dir); !slices.Equal(got, []string{"demo.mp4"}) {
		t.Errorf("files = %v; want only demo.mp4 and no temp dir", got)
	}
}

func TestRunDownloadFailureLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	p, _ := fakePipeline(io.Discard)
	p.download = func(context.Context, clipOpts, string, bool) (string, error) { return "", errors.New("network down") }
	if err := p.run(context.Background(), baseOpts(dir, "gif")); err == nil {
		t.Fatal("expected error")
	}
	if got := dirNames(t, dir); len(got) != 0 {
		t.Errorf("leftover files: %v", got)
	}
}

func TestRunCancelledContextCleansUp(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	p, _ := fakePipeline(io.Discard)
	realDownload := p.download
	p.download = func(c context.Context, o clipOpts, tmp string, a bool) (string, error) {
		f, err := realDownload(c, o, tmp, a)
		cancel() // Ctrl+C arrives right after the download
		return f, err
	}
	if err := p.run(ctx, baseOpts(dir, "gif", "mp4")); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want context.Canceled", err)
	}
	if got := dirNames(t, dir); len(got) != 0 {
		t.Errorf("leftover files: %v", got)
	}
}

func TestYtdlpDownloadArgs(t *testing.T) {
	o := clipOpts{URL: "-evil-url", Start: 83.5, End: 90}
	args := ytdlpDownloadArgs(o, "/pinned/ffmpeg", "/tmp/x", true)
	if n := len(args); n < 2 || args[n-2] != "--" || args[n-1] != "-evil-url" {
		t.Errorf("URL must follow `--`: %q", args)
	}
	if argAfter(args, "--download-sections") != "*83.5-90" {
		t.Errorf("section = %q", argAfter(args, "--download-sections"))
	}
	if argAfter(args, "--ffmpeg-location") != "/pinned/ffmpeg" {
		t.Error("--ffmpeg-location must point at the pinned ffmpeg")
	}
	if argAfter(args, "-f") != "bv*+ba/b" || !slices.Contains(args, "--force-keyframes-at-cuts") {
		t.Errorf("args = %q", args)
	}
	if got := argAfter(ytdlpDownloadArgs(o, "f", "/tmp/x", false), "-f"); got != "bv*/b" {
		t.Errorf("video-only format = %q", got)
	}
}

func TestYtdlpTitleArgs(t *testing.T) {
	args := ytdlpTitleArgs("-evil-url")
	if n := len(args); n < 2 || args[n-2] != "--" || args[n-1] != "-evil-url" {
		t.Errorf("URL must follow `--`: %q", args)
	}
}

func TestPickClipFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "clip.mkv.part"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "clip.mp4"), nil, 0o644)
	got, err := pickClipFile(dir)
	if err != nil || filepath.Base(got) != "clip.mp4" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := pickClipFile(t.TempDir()); err == nil {
		t.Error("expected error when yt-dlp produced nothing")
	}
}

func TestYtdlpIgnoresUserConfig(t *testing.T) {
	for label, args := range map[string][]string{
		"title":    ytdlpTitleArgs("https://x"),
		"download": ytdlpDownloadArgs(clipOpts{URL: "https://x", Start: 1, End: 2}, "ffmpeg", "tmp", false),
	} {
		if !slices.Contains(args, "--ignore-config") {
			t.Errorf("%s args must include --ignore-config so a user's yt-dlp config cannot change a pinned run: %q", label, args)
		}
	}
}
