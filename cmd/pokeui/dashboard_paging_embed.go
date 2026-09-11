package main

import "bytes"

// The paging shim owns dashboard transport and therefore must wrap fetch before
// ui.js performs its first refresh. Keep this transformation separate from the
// rest of operatorIndexPage's decoration so quote escaping cannot silently turn
// the replacement needle into a non-match.
func init() {
	indexHTML = bytes.Replace(
		indexHTML,
		[]byte(`<script src="/ui.js"></script>`),
		[]byte("<script src=\"/dashboard_paging.js\"></script>\n<script src=\"/ui.js\"></script>"),
		1,
	)
}
