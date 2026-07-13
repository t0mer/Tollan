// Package testutil holds small shared test helpers.
package testutil

import (
	"io"
	"log/slog"
	"strconv"
)

// Logger returns a slog.Logger that discards output.
func Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ID returns a deterministic test id.
func ID(i int) string { return "m" + strconv.Itoa(i) }
