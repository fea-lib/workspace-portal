package assets

import "embed"

//go:embed static/htmx-2.0.8.min.js
var HTMXJS []byte

//go:embed static/htmx-ext-sse-2.2.4.min.js
var HTMXSSEJS []byte

//go:embed static/opencode.svg
var OpenCodeSVG []byte

//go:embed static/vscode.svg
var VSCodeSVG []byte

//go:embed static/docs.svg
var DocsSVG []byte

//go:embed static/stop.svg
var StopSVG []byte

//go:embed templates/*.html
var TemplateFS embed.FS
