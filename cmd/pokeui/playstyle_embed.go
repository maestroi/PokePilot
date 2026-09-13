package main

import (
	"bytes"
	_ "embed"
)

//go:embed ui/playstyle.js
var playStyleJS []byte

// Keep this as an inline extension so the legacy console does not need another
// public asset route. It runs after ui.js and augments the existing form/fetch
// path without duplicating the dashboard controller.
func init() {
	const marker = "</body>"
	if !bytes.Contains(indexHTML, []byte(marker)) {
		return
	}
	injected := make([]byte, 0, len(playStyleJS)+96)
	injected = append(injected, []byte(`<script id="play-style-script">`)...)
	injected = append(injected, playStyleJS...)
	injected = append(injected, []byte("</script>\n</body>")...)
	indexHTML = bytes.Replace(indexHTML, []byte(marker), injected, 1)
}
