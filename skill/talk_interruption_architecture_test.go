package skill

import (
	"os"
	"strings"
	"testing"
)

// Farm #2279 exposed a TalkAt ownership gap: the final facing tap can start a
// delayed grass encounter after talkBeside has already completed. Keep that
// tap inside the shared interruption runner so a live battle is resolved and
// the wandering NPC is re-read from scratch instead of leaking into the
// objective finish boundary.
func TestTalkAtFinalFaceKeepsBattleRecoveryOwnership(t *testing.T) {
	data, err := os.ReadFile("interact.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	talkStart := strings.Index(src, "func TalkAt(")
	if talkStart < 0 {
		t.Fatal("TalkAt not found")
	}
	talkEnd := strings.Index(src[talkStart:], "\n// mapObjectSlot resolves")
	if talkEnd < 0 {
		t.Fatal("TalkAt end not found")
	}
	talk := src[talkStart : talkStart+talkEnd]
	if !strings.Contains(talk, "RunInterruptible(m, policy, InterruptibleAction{") ||
		!strings.Contains(talk, "\"TalkAt face live object\"") {
		t.Fatal("TalkAt final facing is no longer owned by the shared interruption runner")
	}

	faceStart := strings.Index(src, "func faceLiveMapObjectWithDecoder")
	if faceStart < 0 {
		t.Fatal("faceLiveMapObjectWithDecoder not found")
	}
	faceEnd := strings.Index(src[faceStart:], "\nfunc liveObjectPosition")
	if faceEnd < 0 {
		t.Fatal("faceLiveMapObjectWithDecoder end not found")
	}
	face := src[faceStart : faceStart+faceEnd]
	if !strings.Contains(face, "errors.Is(err, ErrBattle)") ||
		!strings.Contains(face, "return tx, ty, false, err") {
		t.Fatal("final Face no longer propagates a live battle to TalkAt's interruption runner")
	}
}
