// Command installtools installs every tool listed in deps.json into a
// directory. The test Dockerfile runs it in its own layer so the pinned tools
// are only re-downloaded when deps.json or the installer changes.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"vid2gif/internal/deps"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: installtools <deps.json> <bin dir>")
		os.Exit(2)
	}
	pins, err := deps.Load(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := deps.InstallAll(pins, filepath.Clean(os.Args[2]), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
