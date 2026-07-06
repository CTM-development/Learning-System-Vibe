// Package web embeds the built frontend (web/dist) into the server binary.
// Run `make web` (or `npm run build` in web/) to populate dist; without a
// build the server still starts and serves a "UI not built" notice.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
