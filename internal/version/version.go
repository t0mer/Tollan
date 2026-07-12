// Package version exposes the build-time version metadata for Tollan.
package version

import (
	"fmt"
	"runtime"
)

// These variables are injected at build time via
// -ldflags "-X github.com/t0mer/tollan/internal/version.Version=...".
var (
	// Version is the release version (YYYY.M.PATCH). Defaults to "dev".
	Version = "dev"
	// Commit is the short git SHA of the build.
	Commit = "none"
	// Date is the build timestamp (RFC3339).
	Date = "unknown"
)

// String returns a human-readable one-line version summary.
func String() string {
	return fmt.Sprintf("tollan %s (commit %s, built %s, %s)",
		Version, Commit, Date, runtime.Version())
}

// Info describes the build for API/JSON consumers.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Go      string `json:"go"`
}

// Get returns structured build information.
func Get() Info {
	return Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
		Go:      runtime.Version(),
	}
}
