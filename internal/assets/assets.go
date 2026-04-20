package assets

import "embed"

//go:embed static/htmx-2.0.8.min.js
var HTMXJS []byte

//go:embed static/htmx-ext-sse-2.2.4.min.js
var HTMXSSEJS []byte

//go:embed templates/*.html
var TemplateFS embed.FS
