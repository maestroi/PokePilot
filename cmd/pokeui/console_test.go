package main

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The operator page used to fetch /frame in a tight loop with no delay
// on success. Chrome then spent a third of a core decoding 160x144 PNGs
// as fast as the overlay could answer — one loop per running card.
func TestUIFramePumpIsCapped(t *testing.T) {
	src := string(uiJS)
	m := regexp.MustCompile(`const frameMs = (\d+)`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("ui.js frame pump has no frameMs; unbounded fetch+decode burns the tab")
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("frameMs: %v", err)
	}
	if n < 50 {
		t.Fatalf("frameMs = %d, want >= 50 (20 fps cap; 0 is a tight loop)", n)
	}
}

func TestUIDoesNotKeepPumpingFinishedRuns(t *testing.T) {
	src := string(uiJS)
	// The live pump set is only running cards. A selected done run used
	// to stay in that set and re-decode the same last PNG forever.
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, `sel.status === "done"`) && strings.Contains(line, "want.add") {
			t.Fatal("selected done runs are in the live pump set")
		}
	}
}
