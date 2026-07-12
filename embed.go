// Package tollan is the module root. It embeds the build artifacts that must
// ship inside the single binary: the canonical OpenAPI spec and the built web
// UI. Keeping the embeds here (rather than in an internal package) lets a single
// package reach both the api/ and web/dist/ trees.
package tollan

import (
	"embed"
	"io/fs"
)

//go:embed api/openapi.yaml
var openAPISpec []byte

//go:embed all:web/dist
var webDist embed.FS

// OpenAPISpec returns the raw bytes of the canonical OpenAPI 3 specification.
func OpenAPISpec() []byte {
	return openAPISpec
}

// WebDistFS returns the built web UI rooted at web/dist. Before a frontend build
// this contains only the placeholder index.html.
func WebDistFS() (fs.FS, error) {
	return fs.Sub(webDist, "web/dist")
}
