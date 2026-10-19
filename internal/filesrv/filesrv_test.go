package filesrv

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func get(s *Server, target string, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func setup(t *testing.T) (*Server, string) {
	dir := t.TempDir()
	s := New()
	s.Allow(dir)
	return s, dir
}

func TestServesFileUnderAllowedFolder(t *testing.T) {
	s, dir := setup(t)
	p := write(t, dir, "a.gif", "GIF89a-data")
	u, ok := s.URL(p)
	if !ok {
		t.Fatal("URL not ok")
	}
	w := get(s, u)
	if w.Code != 200 || w.Body.String() != "GIF89a-data" {
		t.Fatalf("code=%d body=%q", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff")
	}
}

func TestRoundTripAwkwardNames(t *testing.T) {
	s, dir := setup(t)
	for _, n := range []string{"a b#c.gif", "100%.gif", "ünï 動画 &=+;,.webp", "x%2Fy.mp4"} {
		p := write(t, dir, n, "data-"+n)
		u, ok := s.URL(p)
		if !ok {
			t.Fatalf("%s: URL not ok", n)
		}
		if strings.ContainsAny(strings.TrimPrefix(u, "/out/"), "#? \\") {
			t.Errorf("%s: unescaped chars in %q", n, u)
		}
		w := get(s, u)
		if w.Code != 200 || w.Body.String() != "data-"+n {
			t.Errorf("%s: code=%d url=%s", n, w.Code, u)
		}
	}
}

// A Windows-style path must survive URL -> ServeHTTP parsing (escaping only).
func TestWindowsPathEscapesAndParses(t *testing.T) {
	esc := escapePath(`C:\Users\me\Videos\Clips\a b#c.gif`)
	r := httptest.NewRequest(http.MethodGet, "/out/"+esc, nil)
	got, ok := pathFromRequest(r)
	if !ok || got != `C:\Users\me\Videos\Clips\a b#c.gif` {
		t.Fatalf("got %q ok=%v (esc %s)", got, ok, esc)
	}
}

func TestRejectsPathOutsideAllowedFolders(t *testing.T) {
	s, _ := setup(t)
	other := write(t, t.TempDir(), "o.gif", "x")
	if _, ok := s.URL(other); ok {
		t.Error("URL ok for outside file")
	}
	if w := get(s, "/out/"+escapePath(other)); w.Code != 404 {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestRejectsSiblingPrefixFolder(t *testing.T) {
	base := t.TempDir()
	allowed := filepath.Join(base, "clips")
	sib := filepath.Join(base, "clips-evil")
	for _, d := range []string{allowed, sib} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s := New()
	s.Allow(allowed)
	p := write(t, sib, "a.gif", "x")
	if w := get(s, "/out/"+escapePath(p)); w.Code != 404 {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestRejectsTraversal(t *testing.T) {
	s, dir := setup(t)
	write(t, filepath.Dir(dir), "secret.gif", "s")
	for _, u := range []string{
		"/out/..%2f..%2fsecret.gif",
		"/out/" + escapePath(filepath.Join(dir, "..", "secret.gif")),
		"/out/" + escapePath(dir+string(filepath.Separator)+".."+string(filepath.Separator)+"secret.gif"),
	} {
		if w := get(s, u); w.Code != 404 {
			t.Errorf("%s: code=%d", u, w.Code)
		}
	}
}

func TestRejectsNonMediaExtension(t *testing.T) {
	s, dir := setup(t)
	for _, n := range []string{"n.txt", "n.exe", "noext", "n.gif.txt"} {
		p := write(t, dir, n, "x")
		if w := get(s, "/out/"+escapePath(p)); w.Code != 404 {
			t.Errorf("%s: code=%d", n, w.Code)
		}
		if _, ok := s.URL(p); ok {
			t.Errorf("%s: URL ok", n)
		}
	}
}

func TestExtensionCaseInsensitive(t *testing.T) {
	s, dir := setup(t)
	p := write(t, dir, "UP.GIF", "x")
	if w := get(s, issue(t, s, p)); w.Code != 200 {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestDirectoryNamedLikeMediaIs404(t *testing.T) {
	s, dir := setup(t)
	d := filepath.Join(dir, "d.gif")
	if err := os.Mkdir(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if w := get(s, "/out/"+escapePath(d)); w.Code != 404 {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestSymlinkEscapeRejected(t *testing.T) {
	s, dir := setup(t)
	outside := write(t, t.TempDir(), "o.gif", "x")
	link := filepath.Join(dir, "link.gif")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if w := get(s, "/out/"+escapePath(link)); w.Code != 404 {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestSymlinkInsideAllowed(t *testing.T) {
	s, dir := setup(t)
	target := write(t, dir, "t.gif", "ok")
	link := filepath.Join(dir, "l.gif")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if w := get(s, issue(t, s, link)); w.Code != 200 {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestRangeRequestGets206(t *testing.T) {
	s, dir := setup(t)
	p := write(t, dir, "v.mp4", "0123456789")
	w := get(s, issue(t, s, p), "Range", "bytes=2-4")
	if w.Code != http.StatusPartialContent || w.Body.String() != "234" {
		t.Fatalf("code=%d body=%q", w.Code, w.Body.String())
	}
}

func TestVanishedFileIs404(t *testing.T) {
	s, dir := setup(t)
	p := write(t, dir, "v.gif", "x")
	u, _ := s.URL(p)
	os.Remove(p)
	if w := get(s, u); w.Code != 404 {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestContentTypes(t *testing.T) {
	s, dir := setup(t)
	for ext, want := range map[string]string{
		".gif": "image/gif", ".webp": "image/webp", ".mp4": "video/mp4", ".mov": "video/quicktime", ".MP4": "video/mp4",
	} {
		p := write(t, dir, "f"+ext, "x")
		w := get(s, issue(t, s, p))
		if got := w.Header().Get("Content-Type"); got != want {
			t.Errorf("%s: got %q want %q", ext, got, want)
		}
	}
}

func TestMethods(t *testing.T) {
	s, dir := setup(t)
	p := write(t, dir, "a.gif", "x")
	u := issue(t, s, p)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest(http.MethodPost, u, nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST code=%d", w.Code)
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest(http.MethodHead, u, nil))
	if w.Code != 200 {
		t.Errorf("HEAD code=%d", w.Code)
	}
}

func TestBadPrefixAndRelative(t *testing.T) {
	s, _ := setup(t)
	for _, u := range []string{"/other/x.gif", "/out/", "/out/a.gif", "/out/%zz"} {
		r := httptest.NewRequest(http.MethodGet, "/out/x", nil)
		r.URL.Path, r.URL.RawPath = u, ""
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Errorf("%s: code=%d", u, w.Code)
		}
	}
}

func TestConcurrent(t *testing.T) {
	s, dir := setup(t)
	p := write(t, dir, "a.gif", "x")
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				s.Allow(dir)
				u, _ := s.URL(p)
				get(s, u)
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

// hooked returns a server whose filesystem hooks count calls.
func hooked(t *testing.T) (*Server, string, *int) {
	s, dir := setup(t)
	n := new(int)
	s.evalSymlinks = func(p string) (string, error) { *n++; return filepath.EvalSymlinks(p) }
	s.stat = func(p string) (os.FileInfo, error) { *n++; return os.Stat(p) }
	return s, dir, n
}

func TestOutsidePathsNeverTouchDisk(t *testing.T) {
	s, _, n := hooked(t)
	outside := filepath.Join(t.TempDir(), "o.gif")
	for _, p := range []string{
		outside,
		`\host\share\x.gif`,
		`\?\C:\Windows\x.gif`,
		`\.\pipe\x.gif`,
		`//host/share/x.gif`,
	} {
		if w := get(s, "/out/"+escapePath(p)); w.Code != 404 {
			t.Errorf("%s: code=%d", p, w.Code)
		}
	}
	if *n != 0 {
		t.Fatalf("filesystem touched %d times for outside paths", *n)
	}
}

func TestInsidePathDoesTouchDisk(t *testing.T) { // guards the hook test above
	s, dir, n := hooked(t)
	p := write(t, dir, "a.gif", "x")
	if w := get(s, issue(t, s, p)); w.Code != 200 || *n == 0 {
		t.Fatalf("code=%d touched=%d", w.Code, *n)
	}
}

func TestAllowRefusesEmptyBlankAndRoot(t *testing.T) {
	s := New()
	cwd, _ := os.Getwd()
	root := filepath.VolumeName(cwd) + string(filepath.Separator)
	for _, d := range []string{"", "   ", "\t", root} {
		if s.Allow(d) {
			t.Errorf("Allow(%q) accepted", d)
		}
	}
	// Nothing was allowed, so the working directory is not served.
	p := write(t, t.TempDir(), "a.gif", "x")
	if _, ok := s.URL(p); ok {
		t.Error("URL ok with no allowed folders")
	}
	if !s.Allow(t.TempDir()) {
		t.Error("real folder refused")
	}
}

func TestRejectsAlternateDataStream(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ADS is Windows-only")
	}
	s, dir := setup(t)
	p := filepath.Join(dir, "x.txt:evil.gif")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Skipf("ADS unsupported: %v", err)
	}
	if w := get(s, "/out/"+escapePath(p)); w.Code != 404 {
		t.Fatalf("code=%d", w.Code)
	}
	if _, ok := s.URL(p); ok {
		t.Error("URL ok for ADS")
	}
}

// issue asks the server for p's URL, as the app does for a made file.
func issue(t *testing.T, s *Server, p string) string {
	t.Helper()
	u, ok := s.URL(p)
	if !ok {
		t.Fatalf("URL(%s) refused", p)
	}
	return u
}

func TestNeverIssuedFileInAllowedFolderIs404(t *testing.T) {
	s, dir := setup(t)
	p := write(t, dir, "old.gif", "x")
	if w := get(s, "/out/"+escapePath(p)); w.Code != 404 {
		t.Fatalf("never-issued file: code=%d", w.Code)
	}
	if s.Issued(p) {
		t.Fatal("Issued true before URL")
	}
	u := issue(t, s, p)
	if !s.Issued(p) {
		t.Fatal("Issued false after URL")
	}
	if w := get(s, u); w.Code != 200 {
		t.Fatalf("issued file: code=%d", w.Code)
	}
}
