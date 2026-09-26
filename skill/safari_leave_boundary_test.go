package skill

import (
	"os"
	"strings"
	"testing"
)

// Farm #1943: leaving the Safari Zone early stopped on the gate's join
// trigger, so "Would you like to join the hunt?" opened after the leave had
// already reported success. With no money for another session, SafariCatch
// returned with that YES/NO still open. Every successful leave must settle
// the re-join prompt itself.
func TestSafariLeaveDeclinesRejoinPromptOnEveryExit(t *testing.T) {
	srcBytes, err := os.ReadFile("fuchsia_story.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(srcBytes)
	start := strings.Index(src, "func leaveSafariZoneIfNeeded")
	end := strings.Index(src, "func declineSafariRejoinPrompt")
	if start < 0 || end <= start {
		t.Fatal("leaveSafariZoneIfNeeded / declineSafariRejoinPrompt not found")
	}
	leave := src[start:end]
	if got := strings.Count(leave, "return declineSafariRejoinPrompt(m)"); got != 2 {
		t.Fatalf("leave exits through declineSafariRejoinPrompt %d time(s), want 2 (travel-exit and gate-walk paths)", got)
	}
	if strings.Contains(leave, "eventInSafariZone) {\n\t\t\treturn nil") {
		t.Fatal("a leave path returns success without settling the gate's re-join prompt")
	}
}
