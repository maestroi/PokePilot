package main

import "fmt"

// policyRawWriter is the prompt-side writer for the live Raw Exchange panel.
// It puts the active gameplay policy ahead of the verbatim prompt so the mode
// remains inspectable even when a huge Observation is clipped. Unlike the
// generic heartbeat clip, it preserves both the prompt head and tail: the head
// contains the system instructions and the tail contains Offered objectives.
type policyRawWriter struct {
	snap           *heartbeatSnap
	playStyle      string
	riskTolerance  string
	wildEncounters string
}

func (w policyRawWriter) Write(p []byte) (int, error) {
	prefix := fmt.Sprintf(
		"=== run policy ===\nplay_style=%s\nrisk_tolerance=%s\nwild_encounters=%s\n\n",
		w.playStyle,
		w.riskTolerance,
		w.wildEncounters,
	)
	budget := maxRawPrompt - len(prefix)
	if budget < 0 {
		budget = 0
	}
	w.snap.storeRaw(prefix+clipRawPromptMiddle(string(p), budget), true)
	return len(p), nil
}

func clipRawPromptMiddle(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	const marker = "\n… clipped middle …\n"
	if n <= len(marker) {
		return s[:n]
	}
	remaining := n - len(marker)
	head := remaining / 2
	tail := remaining - head
	return s[:head] + marker + s[len(s)-tail:]
}
