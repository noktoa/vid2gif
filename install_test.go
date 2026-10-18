package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func serve(t *testing.T, body []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestDownloadVerifiedOK(t *testing.T) {
	body := []byte("hello binary")
	dest := filepath.Join(t.TempDir(), "x")
	if err := downloadVerified(serve(t, body), sha(body), dest); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, body) {
		t.Errorf("content = %q", got)
	}
}

func TestDownloadVerifiedMismatchDeletesFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "x")
	err := downloadVerified(serve(t, []byte("tampered")), sha([]byte("original")), dest)
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Errorf("err = %v", err)
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("file must be removed after a hash mismatch")
	}
}

func TestDownloadVerifiedHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "x")
	if err := downloadVerified(srv.URL, "00", dest); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v", err)
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("no file should remain after an HTTP error")
	}
}

func TestExtractBinaryZip(t *testing.T) {
	arc := filepath.Join(t.TempDir(), "a.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"ffmpeg-x-win64-gpl/bin/ffprobe.exe": "probe",
		"ffmpeg-x-win64-gpl/doc/ffmpeg.exe":  "wrong dir",
		"ffmpeg-x-win64-gpl/bin/ffmpeg.exe":  "the real one",
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	os.WriteFile(arc, buf.Bytes(), 0o644)

	dest := filepath.Join(t.TempDir(), "ffmpeg.exe")
	if err := extractBinary(arc, "zip", "ffmpeg.exe", dest); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "the real one" {
		t.Errorf("extracted %q", got)
	}
	if err := extractBinary(arc, "zip", "missing.exe", dest); err == nil {
		t.Error("expected error for a missing entry")
	}
}

func TestExtractBinaryTarXz(t *testing.T) {
	arc := filepath.Join(t.TempDir(), "a.tar.xz")
	var buf bytes.Buffer
	xw, _ := xz.NewWriter(&buf)
	tw := tar.NewWriter(xw)
	for _, e := range []struct{ name, body string }{
		{"ffmpeg-x-linux64-gpl/bin/ffprobe", "probe"},
		{"ffmpeg-x-linux64-gpl/bin/ffmpeg", "the real one"},
	} {
		tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(e.body))
	}
	tw.Close()
	xw.Close()
	os.WriteFile(arc, buf.Bytes(), 0o644)

	dest := filepath.Join(t.TempDir(), "ffmpeg")
	if err := extractBinary(arc, "tar.xz", "ffmpeg", dest); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "the real one" {
		t.Errorf("extracted %q", got)
	}
	if err := extractBinary(arc, "rar", "ffmpeg", dest); err == nil {
		t.Error("expected error for an unknown archive kind")
	}
}

// A failed install (here: a SHA-256 mismatch) must leave a previously
// installed binary untouched.
func TestInstallToolFailureKeepsExistingBinary(t *testing.T) {
	binDir := t.TempDir()
	dest := filepath.Join(binDir, exeName("yt-dlp"))
	os.WriteFile(dest, []byte("old binary"), 0o755)
	tl := tool{Version: "1", Platforms: map[string]asset{
		platformKey(): {URL: serve(t, []byte("tampered")), SHA256: sha([]byte("original")), Archive: "none"},
	}}
	if err := installTool("yt-dlp", tl, binDir, io.Discard); err == nil {
		t.Fatal("expected a sha256 error")
	}
	if got, _ := os.ReadFile(dest); string(got) != "old binary" {
		t.Errorf("existing binary was damaged: %q", got)
	}
	entries, _ := os.ReadDir(binDir)
	if len(entries) != 1 {
		t.Errorf("temp files left in bin: %v", entries)
	}
}
