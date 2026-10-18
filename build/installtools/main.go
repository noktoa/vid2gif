// Command installtools installs every tool listed in deps.json into a
// directory. The test Dockerfile runs it in its own layer so the pinned tools
// are only re-downloaded when deps.json or the installer changes.
package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

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
	for _, name := range slices.Sorted(maps.Keys(pins)) {
		if err := deps.Install(name, pins[name], filepath.Clean(os.Args[2]), os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", name, err)
			os.Exit(1)
		}
	}
}
