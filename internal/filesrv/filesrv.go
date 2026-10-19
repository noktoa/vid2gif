// Package filesrv serves generated previews (GIF/WebP/MP4/MOV) from allowed
// folders to the desktop WebView. The page chooses the path, so every request
// is treated as hostile: only absolute paths whose real location (symlinks
// resolved) is inside an allowed folder, with a media extension, are served.
package filesrv

import (
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const prefix = "/out/"

// Content types are registered explicitly: the Windows registry may lack
// .webp or map the others to something unexpected.
var mediaTypes = map[string]string{
	".gif":  "image/gif",
	".webp": "image/webp",
	".mp4":  "video/mp4",
	".mov":  "video/quicktime",
}

func init() {
	for ext, typ := range mediaTypes {
		_ = mime.AddExtensionType(ext, typ)
	}
}

// Server serves media files from allowed folders. Safe for concurrent use.
type Server struct {
	mu   sync.RWMutex
	dirs []string
}

func New() *Server { return &Server{} }

// Allow adds a folder the page may read from.
func (s *Server) Allow(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	abs = filepath.Clean(abs)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.dirs {
		if d == abs {
			return
		}
	}
	s.dirs = append(s.dirs, abs)
}

func (s *Server) allowed() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.dirs...)
}

func escapePath(p string) string { return url.PathEscape(p) }

// URL returns the /out/ URL for path, or false when it is not a media file
// lexically under an allowed folder. The file need not exist yet.
func (s *Server) URL(path string) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil || !isMedia(abs) {
		return "", false
	}
	abs = filepath.Clean(abs)
	for _, d := range s.allowed() {
		if within(d, abs) {
			return prefix + escapePath(abs), true
		}
	}
	return "", false
}

func isMedia(p string) bool {
	_, ok := mediaTypes[strings.ToLower(filepath.Ext(p))]
	return ok
}

// within reports whether p is dir or below it (filepath.Rel is case-insensitive
// on Windows and errors across volumes).
func within(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// pathFromRequest extracts the unescaped path from /out/<escaped path>.
func pathFromRequest(r *http.Request) (string, bool) {
	esc, ok := strings.CutPrefix(r.URL.EscapedPath(), prefix)
	if !ok {
		return "", false
	}
	p, err := url.PathUnescape(esc)
	return p, err == nil
}

// resolve returns the real path of a requested file if it may be served.
func (s *Server) resolve(p string) (string, bool) {
	if !filepath.IsAbs(p) || !isMedia(p) {
		return "", false
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(p))
	if err != nil || !isMedia(real) {
		return "", false
	}
	if fi, err := os.Stat(real); err != nil || !fi.Mode().IsRegular() {
		return "", false
	}
	for _, d := range s.allowed() {
		if rd, err := filepath.EvalSymlinks(d); err == nil && within(rd, real) {
			return real, true
		}
	}
	return "", false
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p, ok := pathFromRequest(r)
	if ok {
		p, ok = s.resolve(p)
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, p)
}
