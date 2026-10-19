package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseClipArgsDefaults(t *testing.T) {
	o, err := parseClipArgs([]string{"https://x/y", "--start", "1:23"})
	if err != nil {
		t.Fatal(err)
	}
	if o.URL != "https://x/y" || o.Start != 83 || o.End != 0 || !slices.Equal(o.Formats, []string{"gif"}) ||
		o.OutDir != "./gifs" || o.Quality != "medium" || o.Name != "" || o.Overwrite || o.NoPin || o.FullSource {
		t.Errorf("unexpected defaults: %+v", o)
	}
}

func TestParseClipArgsAllFlagsAfterURL(t *testing.T) {
	o, err := parseClipArgs([]string{"https://x/y", "--start", "10", "--end", "1:00", "--format", "gif,mp4",
		"--out", "D", "--name", "demo", "--quality", "high", "--overwrite", "--no-pin", "--full-source"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Start != 10 || o.End != 60 || !slices.Equal(o.Formats, []string{"gif", "mp4"}) || o.OutDir != "D" ||
		o.Name != "demo" || o.Quality != "high" || !o.Overwrite || !o.NoPin || !o.FullSource {
		t.Errorf("unexpected opts: %+v", o)
	}
}

func TestParseClipArgsFullSource(t *testing.T) {
	o, err := parseClipArgs([]string{"https://x/y", "--start", "1", "--full-source"})
	if err != nil || !o.FullSource {
		t.Errorf("FullSource = %v, err = %v; want true", o.FullSource, err)
	}
}

func TestParseClipArgsUsageErrors(t *testing.T) {
	cases := map[string][]string{
		"no args":             {},
		"url not first":       {"--start", "1", "https://x"},
		"missing start":       {"https://x"},
		"bad start":           {"https://x", "--start", "abc"},
		"bad end":             {"https://x", "--start", "1", "--end", "x"},
		"end before start":    {"https://x", "--start", "10", "--end", "5"},
		"end equals start":    {"https://x", "--start", "10", "--end", "10"},
		"bad format":          {"https://x", "--start", "1", "--format", "avi"},
		"bad quality":         {"https://x", "--start", "1", "--quality", "ultra"},
		"unknown flag":        {"https://x", "--start", "1", "--bogus"},
		"extra positional":    {"https://x", "--start", "1", "extra"},
		"name with slash":     {"https://x", "--start", "1", "--name", "../evil"},
		"name with backslash": {"https://x", "--start", "1", "--name", `..\evil`},
	}
	for label, args := range cases {
		_, err := parseClipArgs(args)
		if err == nil || errors.Is(err, flag.ErrHelp) {
			t.Errorf("%s: err = %v; want a usage error", label, err)
		}
	}
}

func TestParseClipArgsHelp(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"https://x", "-h"}} {
		if _, err := parseClipArgs(args); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%v: err = %v; want flag.ErrHelp", args, err)
		}
	}
}

func TestHelpListsCommandsFlagsAndExample(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		var out, errb bytes.Buffer
		if code := realMain(args, &out, &errb); code != 0 {
			t.Fatalf("realMain(%v) = %d", args, code)
		}
		for _, want := range []string{
			"vid2gif clip <url>", "vid2gif deps install", "Example:", "--start 1:23",
			"-start", "-end", "-format", "-out", "-name", "-quality", "-overwrite", "-no-pin", "-full-source",
		} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("realMain(%v) help is missing %q:\n%s", args, want, out.String())
			}
		}
	}
}

func TestRealMainExitCodes(t *testing.T) {
	cases := []struct {
		args []string
		want int
	}{
		{nil, 2},
		{[]string{"bogus"}, 2},
		{[]string{"deps"}, 2},
		{[]string{"deps", "uninstall"}, 2},
		{[]string{"clip"}, 2},
		{[]string{"clip", "https://x", "--start", "abc"}, 2},
		{[]string{"clip", "https://x", "--start", "5", "--end", "1"}, 2},
		{[]string{"clip", "-h"}, 0},
		{[]string{"help"}, 0},
	}
	for _, c := range cases {
		var out, errb bytes.Buffer
		if got := realMain(c.args, &out, &errb); got != c.want {
			t.Errorf("realMain(%v) = %d; want %d (stderr: %s)", c.args, got, c.want, errb.String())
		}
	}
}

func TestLoadEnv(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("# comment\nVID2GIF_BIN=from-dotenv\n"), 0o644)

	t.Setenv("VID2GIF_BIN", "") // registers cleanup, then really unset it
	os.Unsetenv("VID2GIF_BIN")
	loadEnv(dir)
	if got := os.Getenv("VID2GIF_BIN"); got != "from-dotenv" {
		t.Errorf("VID2GIF_BIN = %q; want the .env value", got)
	}

	t.Setenv("VID2GIF_BIN", "from-real-env")
	loadEnv(dir)
	if got := os.Getenv("VID2GIF_BIN"); got != "from-real-env" {
		t.Errorf("VID2GIF_BIN = %q; a real environment variable must win over .env", got)
	}

	loadEnv(t.TempDir()) // no .env file: must be silent
}
