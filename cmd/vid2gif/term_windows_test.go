//go:build windows

package main

import (
	"os"
	"testing"
)

// NUL is a character device but not a console: no bar, no held-back output.
func TestIsTerminalFalseForNUL(t *testing.T) {
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || st.Mode()&os.ModeCharDevice == 0 {
		t.Skipf("NUL is not a character device here (%v, %v)", st, err)
	}
	if isTerminal(f) {
		t.Error("NUL was taken for a terminal")
	}
}
