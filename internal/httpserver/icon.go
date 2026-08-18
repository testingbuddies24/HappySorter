package httpserver

import (
	_ "embed"
	"net/http"
)

// iconSVG is the app icon: a kawaii smirk face on the brand indigo
// gradient tile — wink on one eye, asymmetric smile, cool & monochromatic.
//
//go:embed icon.svg
var iconSVG []byte

// handleIcon serves the embedded app icon as an SVG favicon.
func (s *Server) handleIcon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if _, err := w.Write(iconSVG); err != nil {
		s.logger.Error("writing icon", "error", err)
	}
}
