package skill

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const (
	fakeItemBattle uint16 = 160 + iota
	fakeItemMainVisible
	fakeItemBattleColumn
	fakeItemBattleRow
	fakeItemListVisible
	fakeItemListPosition
	fakeItemQuantity
	fakeItemPromptOpen
	fakeItemPromptCurrent
	fakeItemControllable
	fakeItemMode
)

const fakeItemID uint16 = 0x42

type fakeGen2ItemMachine struct {
	mem    [192]byte
	frames uint64
}

func (m *fakeGen2ItemMachine) Peek8(addr uint16) byte { return m.mem[addr] }

func (m *fakeGen2ItemMachine) PeekInto(addr uint16, dst []byte) {
	copy(dst, m.mem[int(addr):])
}

func (m *fakeGen2ItemMachine) StepFrame() { m.frames++ }

func (m *fakeGen2ItemMachine) StepFrames(n int) {
	if n > 0 {
		m.frames += uint64(n)
	}
}

func (m *fakeGen2ItemMachine) FrameCount() uint64 { return m.frames }

func (m *fakeGen2ItemMachine) Tap(btn emu.Button, hold, gap int) {
	if hold+gap > 0 {
		m.frames += uint64(hold + gap)
	}
	if m.mem[fakeItemPromptOpen] != 0 {
		switch btn {
		case emu.Down:
			m.mem[fakeItemPromptCurrent] = 1
		case emu.Up:
			m.mem[fakeItemPromptCurrent] = 0
		case emu.A:
			if m.mem[fakeItemPromptCurrent] == 1 {
				m.mem[fakeItemPromptOpen] = 0
				if m.mem[fakeItemQuantity] > 0 {
					m.mem[fakeItemQuantity]--
				}
				if m.mem[fakeItemMode] == 1 {
					m.mem[fakeItemBattle] = 0
					m.mem[fakeItemControllable] = 1
				}
			}
		}
		return
	}

	if m.mem[fakeItemListVisible] != 0 {
		switch btn {
		case emu.Down:
			if m.mem[fakeItemListPosition] < 3 {
				m.mem[fakeItemListPosition]++
			}
		case emu.Up:
			if m.mem[fakeItemListPosition] > 0 {
				m.mem[fakeItemListPosition]--
			}
		case emu.A:
			if m.mem[fakeItemListPosition] == 3 {
				m.mem[fakeItemListVisible] = 0
				m.mem[fakeItemPromptOpen] = 1
				m.mem[fakeItemPromptCurrent] = 0
			}
		}
		return
	}

	if m.mem[fakeItemMainVisible] != 0 {
		switch btn {
		case emu.Right:
			if m.mem[fakeItemBattleColumn] < 2 {
				m.mem[fakeItemBattleColumn]++
			}
		case emu.Left:
			if m.mem[fakeItemBattleColumn] > 0 {
				m.mem[fakeItemBattleColumn]--
			}
		case emu.Down:
			m.mem[fakeItemBattleRow] = 1
		case emu.Up:
			m.mem[fakeItemBattleRow] = 0
		case emu.A:
			if m.mem[fakeItemBattleColumn] == 2 && m.mem[fakeItemBattleRow] == 0 {
				m.mem[fakeItemMainVisible] = 0
				m.mem[fakeItemListVisible] = 1
				m.mem[fakeItemListPosition] = 0
			}
		}
	}
}

type fakeGen2ItemInventoryDecoder struct{}

func (fakeGen2ItemInventoryDecoder) DecodeInventory(r game.MemoryReader) game.InventoryState {
	return game.InventoryState{Items: []game.InventoryItem{
		{NativeItemID: 0x10, Quantity: 1},
		{NativeItemID: 0x20, Quantity: 1},
		{NativeItemID: 0x30, Quantity: 1},
		{NativeItemID: fakeItemID, Quantity: int(r.Peek8(fakeItemQuantity))},
	}}
}

type fakeGen2ItemBattleDecoder struct{}

func (fakeGen2ItemBattleDecoder) DecodeBattleState(r game.MemoryReader) (game.BattleState, bool) {
	if r.Peek8(fakeItemBattle) == 0 {
		return game.BattleState{}, false
	}
	return game.BattleState{Kind: game.BattleWild, EnemySpecies: 251, EnemyHP: 12, EnemyMaxHP: 12}, true
}

func (fakeGen2ItemBattleDecoder) DecodeBattleResult(game.MemoryReader) game.BattleResult {
	return game.BattleWon
}

type fakeGen2ItemRuntimeDecoder struct{}

func (fakeGen2ItemRuntimeDecoder) DecodeBattleRuntime(r game.MemoryReader) game.BattleRuntimeState {
	return game.BattleRuntimeState{
		InBattle:     r.Peek8(fakeItemBattle) != 0,
		Controllable: r.Peek8(fakeItemControllable) != 0,
		NativeMapID:  0x123,
		X:            7,
		Y:            9,
	}
}

type fakeGen2ItemBattleMenuDecoder struct{}

func (fakeGen2ItemBattleMenuDecoder) DecodeBattleMainMenu(r game.MemoryReader) game.BattleMainMenuState {
	if r.Peek8(fakeItemMainVisible) == 0 {
		return game.BattleMainMenuState{}
	}
	return game.BattleMainMenuState{
		Visible: true,
		Cursor: game.BattleMenuPosition{
			Column: int(r.Peek8(fakeItemBattleColumn)),
			Row:    int(r.Peek8(fakeItemBattleRow)),
		},
	}
}

func (fakeGen2ItemBattleMenuDecoder) BattleMainMenuEntryPosition(entry game.BattleMenuEntry) (game.BattleMenuPosition, bool) {
	switch entry {
	case game.BattleMenuItems:
		// Deliberately unlike Gen I's left-column, second-row ITEM entry.
		return game.BattleMenuPosition{Column: 2, Row: 0}, true
	case game.BattleMenuFight:
		return game.BattleMenuPosition{Column: 0, Row: 1}, true
	default:
		return game.BattleMenuPosition{}, false
	}
}

type fakeGen2ItemListDecoder struct{}

func (fakeGen2ItemListDecoder) DecodeListMenu(r game.MemoryReader) game.ListMenuState {
	if r.Peek8(fakeItemListVisible) == 0 {
		return game.ListMenuState{}
	}
	return game.ListMenuState{
		Visible:  true,
		Kind:     game.ListMenuItems,
		Position: int(r.Peek8(fakeItemListPosition)),
	}
}

type fakeGen2ItemMenuDecoder struct{}

func (fakeGen2ItemMenuDecoder) DecodeMenuCursor(r game.MemoryReader) game.MenuCursorState {
	return game.MenuCursorState{Current: int(r.Peek8(fakeItemPromptCurrent)), Max: 1}
}

func (fakeGen2ItemMenuDecoder) DecodeTwoOption(r game.MemoryReader) (game.TwoOptionState, bool) {
	if r.Peek8(fakeItemPromptOpen) == 0 {
		return game.TwoOptionState{}, false
	}
	return game.TwoOptionState{Current: int(r.Peek8(fakeItemPromptCurrent))}, true
}

func (fakeGen2ItemMenuDecoder) DecodeStartMenu(game.MemoryReader) game.StartMenuState {
	return game.StartMenuState{}
}

func (fakeGen2ItemMenuDecoder) StartMenuEntryIndex(game.MemoryReader, game.StartMenuEntry) (int, bool) {
	return 0, false
}

type fakeGen2PromptDecoder struct {
	kind game.PromptKind
}

func (d fakeGen2PromptDecoder) DecodePrompt(r game.MemoryReader) game.PromptState {
	if r.Peek8(fakeItemPromptOpen) == 0 {
		return game.PromptState{}
	}
	return game.PromptState{Visible: true, Kind: d.kind}
}

func fakeGen2CaptureExecution() captureExecutionSemantics {
	return captureExecutionSemantics{
		battle:     fakeGen2ItemBattleDecoder{},
		runtime:    fakeGen2ItemRuntimeDecoder{},
		battleMenu: fakeGen2ItemBattleMenuDecoder{},
		menu:       fakeGen2ItemMenuDecoder{},
		prompt:     fakeGen2PromptDecoder{kind: game.PromptNickname},
	}
}

func TestGenericUseItemUsesProfileMenusInventoryAndPrompt(t *testing.T) {
	m := &fakeGen2ItemMachine{}
	m.mem[fakeItemBattle] = 1
	m.mem[fakeItemMainVisible] = 1
	m.mem[fakeItemBattleColumn] = 0
	m.mem[fakeItemBattleRow] = 1
	m.mem[fakeItemQuantity] = 2

	err := useItemWithDecoders(
		m,
		fakeItemID,
		fakeGen2ItemInventoryDecoder{},
		fakeGen2ItemBattleDecoder{},
		fakeGen2ItemRuntimeDecoder{},
		fakeGen2ItemBattleMenuDecoder{},
		fakeGen2ItemListDecoder{},
		fakeGen2ItemMenuDecoder{},
		fakeGen2PromptDecoder{kind: game.PromptNickname},
	)
	if err != nil {
		t.Fatalf("use fake Gen-II item: %v", err)
	}
	if got := m.mem[fakeItemQuantity]; got != 1 {
		t.Fatalf("item quantity = %d, want 1 after verified consumption", got)
	}
	if m.mem[fakeItemPromptOpen] != 0 {
		t.Fatal("nickname prompt remained open")
	}
	if got := m.mem[fakeItemPromptCurrent]; got != 1 {
		t.Fatalf("nickname choice = %d, want NO (1)", got)
	}
	if got := m.mem[fakeItemBattleColumn]; got != 2 {
		t.Fatalf("battle-menu column = %d, want fake Gen-II ITEMS column 2", got)
	}
}

func TestGenericUseItemRefusesUnknownChoicePrompt(t *testing.T) {
	m := &fakeGen2ItemMachine{}
	m.mem[fakeItemBattle] = 1
	m.mem[fakeItemMainVisible] = 1
	m.mem[fakeItemQuantity] = 2

	err := useItemWithDecoders(
		m,
		fakeItemID,
		fakeGen2ItemInventoryDecoder{},
		fakeGen2ItemBattleDecoder{},
		fakeGen2ItemRuntimeDecoder{},
		fakeGen2ItemBattleMenuDecoder{},
		fakeGen2ItemListDecoder{},
		fakeGen2ItemMenuDecoder{},
		fakeGen2PromptDecoder{},
	)
	if err == nil || !strings.Contains(err.Error(), "unexpected choice prompt") {
		t.Fatalf("unknown prompt error = %v, want explicit refusal", err)
	}
	if got := m.mem[fakeItemQuantity]; got != 2 {
		t.Fatalf("unknown prompt consumed item: quantity = %d, want 2", got)
	}
}

func TestGenericCatchThrowDeclinesSemanticNicknamePrompt(t *testing.T) {
	m := &fakeGen2ItemMachine{}
	m.mem[fakeItemBattle] = 1
	m.mem[fakeItemPromptOpen] = 1
	m.mem[fakeItemQuantity] = 1
	m.mem[fakeItemMode] = 1

	ended, err := waitThrowResultWithSemantics(m, fakeGen2CaptureExecution())
	if err != nil {
		t.Fatalf("resolve fake Gen-II throw: %v", err)
	}
	if !ended {
		t.Fatal("throw result returned broken-ball path after prompt ended battle")
	}
	if m.mem[fakeItemBattle] != 0 || m.mem[fakeItemPromptOpen] != 0 {
		t.Fatalf("capture did not settle: battle=%d prompt=%d", m.mem[fakeItemBattle], m.mem[fakeItemPromptOpen])
	}
	if got := m.mem[fakeItemPromptCurrent]; got != 1 {
		t.Fatalf("nickname choice = %d, want NO (1)", got)
	}
}

func TestGenericCatchThrowRecognizesProfileBattleMenu(t *testing.T) {
	m := &fakeGen2ItemMachine{}
	m.mem[fakeItemBattle] = 1
	m.mem[fakeItemMainVisible] = 1
	m.mem[fakeItemBattleColumn] = 2

	ended, err := waitThrowResultWithSemantics(m, fakeGen2CaptureExecution())
	if err != nil {
		t.Fatalf("resolve fake Gen-II broken throw: %v", err)
	}
	if ended {
		t.Fatal("visible semantic battle menu should classify the throw as broken, not ended")
	}
}
