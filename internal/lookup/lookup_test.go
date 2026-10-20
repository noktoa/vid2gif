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
		// A link pasted without its scheme (e.g. copied from the address bar of
		// a browser that hides it) still works: https:// is assumed.
		{"scheme stripped", "www.youtube.com/watch?v=abc youtu.be/xyz music.youtube.com/playlist?list=PL1", []string{"https://www.youtube.com/watch?v=abc", "https://youtu.be/xyz", "https://music.youtube.com/playlist?list=PL1"}},
		{"scheme stripped, bare host", "youtube.com", []string{"https://youtube.com"}},
		{"scheme stripped with port", "a.test:8080/x", []string{"https://a.test:8080/x"}},
		{"upper-case scheme kept as typed", "HTTPS://a.test/x", []string{"HTTPS://a.test/x"}},
		{"same link with and without scheme is one", "https://youtu.be/x youtu.be/x", []string{"https://youtu.be/x"}},
		{"stripped dupes", "youtu.be/x youtu.be/x", []string{"https://youtu.be/x"}},
		{"not links without a scheme", "javascript:alert(1) C:\\x.mp4 mailto:a@b.test localhost word e.g. 1.2 v1.0 a.test@evil user@a.test //a.test/x /etc/passwd ../x.gif a.test:abc/x", nil},
		{"whitespace-split sentence", "see youtu.be/x for the clip.", []string{"https://youtu.be/x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseInput(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseInput(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStartFromLink(t *testing.T) {
	for link, want := range map[string]string{
		"https://youtu.be/abc?t=90":                            "1:30",
		"https://youtu.be/abc?si=xyz&t=90":                     "1:30",
		"https://www.youtube.com/watch?v=abc&t=90s":            "1:30",
		"https://www.youtube.com/watch?v=abc&t=1h2m3s":         "1:02:03",
		"https://youtu.be/abc?t=2m":                            "2:00",
		"https://m.youtube.com/watch?v=abc&t=1m30":             "1:30",
		"https://www.youtube.com/embed/abc?start=45":           "0:45",
		"https://youtu.be/abc?t=3600":                          "1:00:00",
		"https://youtu.be/abc":                                 "",
		"https://youtu.be/abc?t=0":                             "",
		"https://youtu.be/abc?t=soon":                          "",
		"https://youtu.be/abc?t=99999999999999999999999":       "",
		"https://vimeo.com/123?t=90":                           "",
		"https://notyoutube.com/watch?v=abc&t=90":              "",
		"https://www.youtube.com/watch?v=abc&list=PL1&t=1m30s": "1:30",
	} {
		if got := StartFromLink(link); got != want {
			t.Errorf("StartFromLink(%q) = %q, want %q", link, got, want)
		}
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
