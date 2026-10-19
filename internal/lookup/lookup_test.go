package lookup

import (
	"os"
	"reflect"
	"testing"
)

func TestParseInput(t *testing.T) {
	tests := []struct {
		name, in string
		want     []string
	}{
		{"blank", "  \n\t ", nil},
		{"whitespace and commas", "http://a.test/1 http://a.test/2\nhttp://a.test/3,http://a.test/4", []string{"http://a.test/1", "http://a.test/2", "http://a.test/3", "http://a.test/4"}},
		{"trims", "  https://a.test/x  ,, ", []string{"https://a.test/x"}},
		{"drops non-http", "javascript:alert(1) C:\\x.mp4 ftp://a.test/x file:///etc/passwd https://a.test/ok", []string{"https://a.test/ok"}},
		{"drops hostless", "http:// https:///x", nil},
		{"dedupes keeping first order", "https://b.test https://a.test https://b.test", []string{"https://b.test", "https://a.test"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseInput(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseInput(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseSingleVideoLine(t *testing.T) {
	src, vids, err := Parse(fixture(t, "single.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if src != "Pasted links" {
		t.Errorf("source = %q", src)
	}
	if len(vids) != 1 {
		t.Fatalf("got %d videos", len(vids))
	}
	v := vids[0]
	if v.ID != "jNQXAC9IVRw" || v.Title != "Me at the zoo" || v.DurationSec != 19 ||
		v.URL != "https://www.youtube.com/watch?v=jNQXAC9IVRw" || v.ThumbnailURL == "" || v.Unavailable {
		t.Errorf("unexpected video %+v", v)
	}
}

func TestParseLiveVideoHasNoDuration(t *testing.T) {
	_, vids, err := Parse([]byte(`{"id":"live1","title":"Live now","webpage_url":"https://www.youtube.com/watch?v=live1","live_status":"is_live"}` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(vids) != 1 || vids[0].DurationSec != 0 || vids[0].Unavailable {
		t.Errorf("got %+v", vids)
	}
}

func TestParsePlaylistLines(t *testing.T) {
	src, vids, err := Parse(fixture(t, "playlist.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if src != "YouTube" {
		t.Errorf("source = %q", src)
	}
	if len(vids) != 3 {
		t.Fatalf("got %d videos", len(vids))
	}
	if vids[0].Unavailable || vids[0].DurationSec != 2895 || vids[0].ThumbnailURL == "" ||
		vids[0].URL != "https://www.youtube.com/watch?v=lVI_J1cbFb4" {
		t.Errorf("entry 0 = %+v", vids[0])
	}
	if !vids[1].Unavailable || vids[1].Reason != "private" {
		t.Errorf("entry 1 = %+v", vids[1])
	}
	if !vids[2].Unavailable || vids[2].Reason != "removed" {
		t.Errorf("entry 2 = %+v", vids[2])
	}
}

func TestParseErrors(t *testing.T) {
	if _, _, err := Parse([]byte("not json\n")); err == nil {
		t.Error("want error for invalid JSON")
	}
	if _, _, err := Parse([]byte("\n \n")); err == nil {
		t.Error("want error when yt-dlp printed no videos")
	}
}

func TestLookupArgs(t *testing.T) {
	got := lookupArgs("https://a.test/v", `C:\tools\deno.exe`)
	want := []string{"--ignore-config", "--no-js-runtimes", "--js-runtimes", `deno:C:\tools\deno.exe`,
		"--flat-playlist", "--dump-json", "--no-playlist", "--", "https://a.test/v"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args = %q", got)
	}
}
