//go:build windows

package main

import (
	"os"
	"syscall"
)

// isConsole is true when f is a console handle, which NUL is not.
func isConsole(f *os.File) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(f.Fd()), &mode) == nil
}
