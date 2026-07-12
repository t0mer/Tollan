// Command tollan-agent is the companion log collector for Tollan: it tails
// files, reads journald / Windows Event Log, and ships to a Tollan server over
// GELF. The full agent lands in a later phase; this stub establishes the binary
// and its version surface so the release matrix builds.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/t0mer/tollan/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	fmt.Fprintln(os.Stderr, "tollan-agent is not implemented yet; see the fleet phase.")
	os.Exit(1)
}
