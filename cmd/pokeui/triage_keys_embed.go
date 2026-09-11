package main

import (
	"bytes"
	_ "embed"
)

//go:embed ui/triage_keys.js
var triageKeysJS []byte

func init() {
	const marker = "</body>"
	if !bytes.Contains(indexHTML, []byte(marker)) {
		return
	}
	injected := make([]byte, 0, len(triageKeysJS)+64)
	injected = append(injected, []byte(`<script id="triage-key-script">`)...)
	injected = append(injected, triageKeysJS...)
	injected = append(injected, []byte("</script>\n</body>")...)
	indexHTML = bytes.Replace(indexHTML, []byte(marker), injected, 1)
}
