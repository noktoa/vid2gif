// Package debuglog is the opt-in diagnostic log: one file per app run, with
// URLs scrubbed of anything that identifies the user or signs a media request.
package debuglog

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	filePrefix = "vid2gif-"
	fileSuffix = ".log"
	keepOld    = 4 // plus this run's file = 5 on disk
)

// Log is safe for concurrent use. A nil *Log is valid and does nothing.
type Log struct {
	dir string

	mu      sync.Mutex
	enabled bool
	logger  *slog.Logger
}

// fileWriter opens, appends and closes on every line: nothing touches disk
// until the first write, and no handle stays open to block deleting the folder.
type fileWriter struct{ dir, name string }

func (w fileWriter) Write(p []byte) (int, error) {
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(filepath.Join(w.dir, w.name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return f.Write(p)
}

// New prunes old logs but creates nothing: the directory and file appear on
// the first Printf after logging is enabled.
func New(dir string, enabled bool) *Log {
	prune(dir)
	name := filePrefix + time.Now().Format("20060102-150405") + fileSuffix
	return &Log{
		dir:     dir,
		enabled: enabled,
		logger:  slog.New(slog.NewTextHandler(fileWriter{dir, name}, nil)),
	}
}

func (l *Log) SetEnabled(on bool) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.enabled = on
	l.mu.Unlock()
}

func (l *Log) Dir() string {
	if l == nil {
		return ""
	}
	return l.dir
}

// Printf writes one timestamped line, redacted. A write failure drops the
// line: diagnostics must never break the app.
func (l *Log) Printf(format string, args ...any) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.enabled {
		return
	}
	l.logger.Info(Redact(fmt.Sprintf(format, args...)))
}

// prune keeps the newest keepOld log files. Names embed the timestamp, so
// lexical order is chronological.
func prune(dir string) {
	files, _ := filepath.Glob(filepath.Join(dir, filePrefix+"*"+fileSuffix))
	slices.Sort(files)
	for len(files) > keepOld {
		os.Remove(files[0])
		files = files[1:]
	}
}

var (
	urlRe   = regexp.MustCompile(`https?://[^\s"'<>]+`)
	paramRe = regexp.MustCompile(`\b(?:ip|expire|sig|lsig|signature)=[^&\s"']*&?`)
	// HLS manifests carry the same signing values as /key/value path segments.
	pathParamRe = regexp.MustCompile(`(?i)/(?:ip|expire|sig|lsig|signature)/[^/?#\s]*`)
	userinfoRe  = regexp.MustCompile(`^(https?://)[^/?#@\s]*@`)
)

// Redact strips the query from every http(s) URL, except that YouTube page
// links keep v= and list= so a log still says which video failed. Userinfo
// (user:pass@) and signing parameters (ip, expire, sig...) are removed, as
// key=value anywhere and as /key/value path segments inside URLs.
func Redact(s string) string {
	s = urlRe.ReplaceAllStringFunc(s, redactURL)
	return paramRe.ReplaceAllString(s, "")
}

func redactURL(u string) string {
	u, _, _ = strings.Cut(u, "#")
	base, query, hasQuery := strings.Cut(u, "?")
	base = userinfoRe.ReplaceAllString(base, "$1")
	base = pathParamRe.ReplaceAllString(base, "")
	if !hasQuery || !isYouTube(base) {
		return base
	}
	q, _ := url.ParseQuery(query)
	var parts []string
	for _, k := range []string{"v", "list"} {
		if v := q.Get(k); v != "" {
			parts = append(parts, k+"="+url.QueryEscape(v))
		}
	}
	if len(parts) == 0 {
		return base
	}
	return base + "?" + strings.Join(parts, "&")
}

func isYouTube(base string) bool {
	p, err := url.Parse(base)
	if err != nil {
		return false
	}
	h := strings.ToLower(p.Hostname())
	return h == "youtu.be" || h == "youtube.com" || strings.HasSuffix(h, ".youtube.com")
}
