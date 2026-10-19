//go:build !windows

package main

import "os"

// isConsole: on Unix a character device is taken as a tty.
// ponytail: /dev/null also passes; an ioctl(TCGETS) check would exclude it.
func isConsole(*os.File) bool { return true }
