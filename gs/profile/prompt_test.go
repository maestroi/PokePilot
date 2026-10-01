package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func liveGSYesNo(mem *fakeMemory) {
	mem[sym.TwoDMenuNumRows] = 2
	mem[sym.TwoDMenuNumCols] = 1
	mem[sym.MenuJoypadFilter] = gen2PadA | gen2PadB
	mem[sym.MenuCursorY] = 1
	mem[sym.MenuCursorX] = 1
}

func TestDecodePromptRecognizesHatchNicknameQuestion(t *testing.T) {
	var mem fakeMemory
	liveGSYesNo(&mem)
	putGSText(&mem, "YES NO Give a nickname to TOGEPI?")
	if got := NewGold().DecodePrompt(&mem); !got.Visible || got.Kind != game.PromptNickname {
		t.Fatalf("prompt = %+v, want visible nickname", got)
	}
}

func TestDecodePromptLeavesOtherYesNoUnclassified(t *testing.T) {
	var mem fakeMemory
	liveGSYesNo(&mem)
	putGSText(&mem, "YES NO Will you take this EGG?")
	if got := NewGold().DecodePrompt(&mem); !got.Visible || got.Kind != game.PromptUnknown {
		t.Fatalf("prompt = %+v, want visible unclassified", got)
	}
	// The question text alone, after the box closed, is not a live choice.
	var stale fakeMemory
	putGSText(&stale, "Give a nickname to TOGEPI?")
	if got := NewGold().DecodePrompt(&stale); got.Visible {
		t.Fatalf("stale text prompt = %+v, want none", got)
	}
}

func TestNamingKeyboardOpen(t *testing.T) {
	var mem fakeMemory
	putGSText(&mem, "TOGEPI'S NICKNAME? A B C D E F G H I lower DEL END")
	if !NewGold().NamingKeyboardOpen(&mem) {
		t.Fatal("naming keyboard not recognized")
	}
	putGSText(&mem, "Give a nickname to TOGEPI?")
	if NewGold().NamingKeyboardOpen(&mem) {
		t.Fatal("dialogue misread as naming keyboard")
	}
}
