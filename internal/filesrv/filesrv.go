// Package filesrv serves the previews the app made (GIF/WebP/MP4/MOV) to the
// desktop WebView. The page chooses the path, so every request is treated as
// hostile: only exact files URL has issued are served, and only while their
// real location (symlinks resolved) is a media file inside an allowed folder.
package filesrv

import (
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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
	mu     sync.RWMutex
	dirs   []string
	issued map[string]bool // cleaned paths URL has returned: the only ones served

	// Filesystem hooks, swapped in tests to prove nothing is touched for
	// paths outside the allowed folders.
	evalSymlinks func(string) (string, error)
	stat         func(string) (os.FileInfo, error)
}

func New() *Server {
	return &Server{issued: map[string]bool{}, evalSymlinks: filepath.EvalSymlinks, stat: os.Stat}
}

// Allow adds a folder the page may read from. It ignores (and returns false
// for) an empty or whitespace-only name and a filesystem root such as "/" or
// a drive root, so a stray empty config value can never expose the working directory
// or a whole drive.
func (s *Server) Allow(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	abs = filepath.Clean(abs)
	if filepath.Dir(abs) == abs {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.dirs {
		if d == abs {
			return true
		}
	}
	s.dirs = append(s.dirs, abs)
	return true
}

func (s *Server) allowed() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.dirs...)
}

func escapePath(p string) string { return url.PathEscape(p) }

// URL returns the /out/ URL for path, or false when it is not a media file
// lexically under an allowed folder. The file need not exist yet. Only paths
// URL has returned are ever served, so call it only for files the app made.
func (s *Server) URL(path string) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil || !isMedia(abs) {
		return "", false
	}
	abs = filepath.Clean(abs)
	for _, d := range s.allowed() {
		if within(d, abs) {
			s.mu.Lock()
			s.issued[abs] = true
			s.mu.Unlock()
			return prefix + escapePath(abs), true
		}
	}
	return "", false
}

// Issued reports whether URL has returned a URL for path (a made file).
func (s *Server) Issued(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.issued[filepath.Clean(abs)]
}

func isMedia(p string) bool {
	// Windows alternate data stream (x.txt:evil.gif) or odd drive-relative name.
	if runtime.GOOS == "windows" && strings.Contains(filepath.Base(p), ":") {
		return false
	}
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
	p = filepath.Clean(p)
	s.mu.RLock()
	issued := s.issued[p]
	s.mu.RUnlock()
	if !issued {
		return "", false
	}
	// Lexical containment next (defence in depth): nothing (UNC shares, pipes, dead drives) is
	// touched on disk for a path outside every allowed folder.
	lexical := false
	for _, d := range s.allowed() {
		if within(d, p) {
			lexical = true
			break
		}
	}
	if !lexical {
		return "", false
	}
	real, err := s.evalSymlinks(p)
	if err != nil || !isMedia(real) {
		return "", false
	}
	for _, d := range s.allowed() {
		if rd, err := s.evalSymlinks(d); err == nil && within(rd, real) {
			if fi, err := s.stat(real); err == nil && fi.Mode().IsRegular() {
				return real, true
			}
			return "", false
		}
	}
	return "", false
}

// ServeHTTP must be mounted directly (not behind http.ServeMux, which cleans the
// decoded path and would mangle the escaped path in the URL).
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
