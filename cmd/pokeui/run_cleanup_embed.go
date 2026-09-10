package main

import (
	"bytes"
	_ "embed"
)

//go:embed ui/run_cleanup.js
var runCleanupJS []byte

func init() {
	const marker = "</body>"
	if !bytes.Contains(indexHTML, []byte(marker)) {
		return
	}
	injected := make([]byte, 0, len(runCleanupJS)+64)
	injected = append(injected, []byte(`<script id="run-cleanup-script">`)...)
	injected = append(injected, runCleanupJS...)
	injected = append(injected, []byte("</script>\n</body>")...)
	indexHTML = bytes.Replace(indexHTML, []byte(marker), injected, 1)
}
