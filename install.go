package main

import (
	"archive/tar"
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
)

// downloadVerified streams url to dest while hashing it. dest is removed on
// any failure, including a SHA-256 mismatch.
func downloadVerified(url, wantSHA, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dest)
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, wantSHA) {
		os.Remove(dest)
		return fmt.Errorf("sha256 mismatch for %s: got %s, want %s", url, got, wantSHA)
	}
	return nil
}

func isBinEntry(name, wantBase string) bool {
	name = strings.ReplaceAll(name, "\\", "/")
	return path.Base(name) == wantBase && path.Base(path.Dir(name)) == "bin"
}

func writeExe(dest string, r io.Reader) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// extractBinary copies the archive entry <anything>/bin/<wantBase> to dest.
func extractBinary(archivePath, kind, wantBase, dest string) error {
	notFound := fmt.Errorf("%s not found in %s", wantBase, archivePath)
	switch kind {
	case "zip":
		zr, err := zip.OpenReader(archivePath)
		if err != nil {
			return err
		}
		defer zr.Close()
		for _, f := range zr.File {
			if f.FileInfo().IsDir() || !isBinEntry(f.Name, wantBase) {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			return writeExe(dest, rc)
		}
		return notFound
	case "tar.xz":
		f, err := os.Open(archivePath)
		if err != nil {
			return err
		}
		defer f.Close()
		xr, err := xz.NewReader(f)
		if err != nil {
			return err
		}
		tr := tar.NewReader(xr)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				return notFound
			}
			if err != nil {
				return err
			}
			if h.Typeflag == tar.TypeReg && isBinEntry(h.Name, wantBase) {
				return writeExe(dest, tr)
			}
		}
	}
	return fmt.Errorf("unknown archive kind %q", kind)
}

// installTool makes binDir/<name> match the pin: no-op if it already does,
// otherwise download, verify, extract and re-check the version.
func installTool(name string, t tool, binDir string, log io.Writer) error {
	a, err := t.assetFor(platformKey())
	if err != nil {
		return err
	}
	exe := exeName(name)
	dest := filepath.Join(binDir, exe)
	if out, err := runVersion(dest, name); err == nil && versionMatches(name, t.Version, out) {
		fmt.Fprintf(log, "%s %s already installed\n", name, t.Version)
		return nil
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	fmt.Fprintf(log, "installing %s %s ...\n", name, t.Version)
	if a.Archive == "none" {
		if err := downloadVerified(a.URL, a.SHA256, dest); err != nil {
			return err
		}
		if err := os.Chmod(dest, 0o755); err != nil {
			return err
		}
	} else {
		tmp := filepath.Join(binDir, "."+name+".download")
		if err := downloadVerified(a.URL, a.SHA256, tmp); err != nil {
			return err
		}
		defer os.Remove(tmp)
		if err := extractBinary(tmp, a.Archive, exe, dest); err != nil {
			return err
		}
	}
	out, err := runVersion(dest, name)
	if err != nil {
		return fmt.Errorf("%s: running version check: %w", dest, err)
	}
	if !versionMatches(name, t.Version, out) {
		return fmt.Errorf("installed %s reports %q, expected %s", name, firstLine(out), t.Version)
	}
	fmt.Fprintf(log, "installed %s\n", dest)
	return nil
}
