// Package templates exposes the bundled draw.io shape libraries.
package templates

import "embed"

// FS contains every draw.io mxlibrary shipped with the server.
//
//go:embed *.xml
var FS embed.FS
