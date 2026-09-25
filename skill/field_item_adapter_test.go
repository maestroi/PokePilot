package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const (
	fakeFieldMedicine uint16 = 0x0123
	fakeFieldEther    uint16 = 0x0456
	fakeFieldRepel    uint16 = 0x0789
)

type fakeFieldItemMachine struct {
	frames uint64
	phase int
	startCursor int
	listPos int
	useCursor int
	partyCursor int
	moveCursor int
	targetSlot int
	items []game.InventoryItem
	party []game.FieldItemPartyMon
	selected uint16
	repel int
}

func newFakeFieldItemMachine() *fakeFieldItemMachine {
	return &fakeFieldItemMachine{
		items: []game.InventoryItem{
			{NativeItemID: 0x0100, Quantity: 1},
			{NativeItemID: fakeFieldMedicine, Quantity: 2},
			{NativeItemID: fakeFieldEther, Quantity: 1},
			{NativeItemID: fakeFieldRepel, Quantity: 1},
		},
		party: []game.FieldItemPartyMon{
			{NativeSpeciesID: 200, Level: 20, HP: 40, MaxHP: 40, Moves: [4]uint16{1, 2}, PP: [4]uint8{10, 10}},
			{NativeSpeciesID: 251, Level: 30, HP: 12, MaxHP: 60, Moves: [4]uint16{10, 11, 12}, PP: [4]uint8{8, 0, 3}},
		},
		targetSlot: -1,
	}
}

func (m *fakeFieldItemMachine) Peek8(uint16) byte { return 0 }
func (m *fakeFieldItemMachine) PeekInto(uint16, []byte) {}
func (m *fakeFieldItemMachine) StepFrame() { m.frames++ }
func (m *fakeFieldItemMachine) StepFrames(n int) { if n > 0 { m.frames += uint64(n) } }
func (m *fakeFieldItemMachine) FrameCount() uint64 { return m.frames }

func (m *fakeFieldItemMachine) Tap(btn emu.Button, hold, gap int) {
	if hold+gap > 0 { m.frames += uint64(hold+gap) }
	switch m.phase {
	case 0:
		if btn == emu.Start { m.phase = 1 }
	case 1:
		switch btn {
		case emu.Down:
			if m.startCursor < 7 { m.startCursor++ }
		case emu.Up:
			if m.startCursor > 0 { m.startCursor-- }
		case emu.A:
			if m.startCursor == 5 { m.phase, m.listPos = 2, 0 }
		}
	case 2:
		switch btn {
		case emu.Down:
			if m.listPos < len(m.items)-1 { m.listPos++ }
		case emu.Up:
			if m.listPos > 0 { m.listPos-- }
		case emu.A:
			m.selected = m.items[m.listPos].NativeItemID
			m.phase, m.useCursor = 3, 0
		}
	case 3:
		switch btn {
		case emu.Down:
			m.useCursor = 1
		case emu.Up:
			m.useCursor = 0
		case emu.A:
			if m.useCursor != 0 { return }
			sem := (fakeFieldItemDecoder{}).FieldItemSemantics(m.selected)
			if sem.RepelSteps > 0 {
				m.consume(m.selected)
				m.repel = sem.RepelSteps
				m.phase = 6
			} else {
				m.phase, m.partyCursor = 4, 0
			}
		}
	case 4:
		switch btn {
		case emu.Down:
			if m.partyCursor < len(m.party)-1 { m.partyCursor++ }
		case emu.Up:
			if m.partyCursor > 0 { m.partyCursor-- }
		case emu.A:
			m.targetSlot = m.partyCursor
			if (fakeFieldItemDecoder{}).FieldItemSemantics(m.selected).SingleMoveTarget {
				m.phase, m.moveCursor = 5, 0
			} else {
				m.applyItem()
			}
		}
	case 5:
		switch btn {
		case emu.Down:
			if m.moveCursor < 2 { m.moveCursor++ }
		case emu.Up:
			if m.moveCursor > 0 { m.moveCursor-- }
		case emu.A:
			m.applyItem()
		}
	case 6:
		if btn == emu.B { m.phase = 0 }
	}
}

func (m *fakeFieldItemMachine) consume(item uint16) {
	for i := range m.items {
		if m.items[i].NativeItemID == item && m.items[i].Quantity > 0 {
			m.items[i].Quantity--
			return
		}
	}
}

func (m *fakeFieldItemMachine) applyItem() {
	if m.targetSlot < 0 || m.targetSlot >= len(m.party) { return }
	sem := (fakeFieldItemDecoder{}).FieldItemSemantics(m.selected)
	if sem.SingleMoveTarget {
		m.party[m.targetSlot].PP[m.moveCursor] += 5
	} else {
		m.party[m.targetSlot].HP = m.party[m.targetSlot].MaxHP
	}
	m.consume(m.selected)
	m.phase = 6
}

type fakeFieldItemDecoder struct{}

func (fakeFieldItemDecoder) FieldItemSemantics(item uint16) game.FieldItemSemantics {
	switch item {
	case fakeFieldEther:
		return game.FieldItemSemantics{SingleMoveTarget: true, PPRestore: true}
	case fakeFieldRepel:
		return game.FieldItemSemantics{RepelSteps: 321}
	default:
		return game.FieldItemSemantics{}
	}
}
func (fakeFieldItemDecoder) PreferredRepels() []uint16 { return []uint16{fakeFieldRepel} }
func (fakeFieldItemDecoder) DecodeFieldItem(r game.MemoryReader) game.FieldItemState {
	m := r.(*fakeFieldItemMachine)
	party := append([]game.FieldItemPartyMon(nil), m.party...)
	s := game.FieldItemState{
		Party: party, RepelSteps: m.repel,
		OverworldReady: m.phase == 0,
		UIOpen: m.phase != 0,
		UsePromptVisible: m.phase == 3,
		UseSelected: m.phase == 3 && m.useCursor == 0,
		MoveMenuVisible: m.phase == 5,
		MoveCursor: game.MenuCursorState{Current:m.moveCursor, Max:2},
		ResultTextActive: m.phase == 6,
		ChoiceVisible: m.phase == 3,
	}
	return s
}

type fakeFieldInventoryDecoder struct{}
func (fakeFieldInventoryDecoder) DecodeInventory(r game.MemoryReader) game.InventoryState {
	m := r.(*fakeFieldItemMachine)
	return game.InventoryState{Items: append([]game.InventoryItem(nil), m.items...)}
}

type fakeFieldMenuDecoder struct{}
func (fakeFieldMenuDecoder) DecodeMenuCursor(r game.MemoryReader) game.MenuCursorState {
	m:=r.(*fakeFieldItemMachine)
	switch m.phase {
	case 1: return game.MenuCursorState{Current:m.startCursor,Max:7}
	case 3: return game.MenuCursorState{Current:m.useCursor,Max:1}
	default: return game.MenuCursorState{}
	}
}
func (fakeFieldMenuDecoder) DecodeTwoOption(r game.MemoryReader) (game.TwoOptionState,bool) {
	m:=r.(*fakeFieldItemMachine)
	if m.phase != 3 { return game.TwoOptionState{},false }
	return game.TwoOptionState{Current:m.useCursor},true
}
func (fakeFieldMenuDecoder) DecodeStartMenu(r game.MemoryReader) game.StartMenuState {
	m:=r.(*fakeFieldItemMachine)
	return game.StartMenuState{Visible:m.phase==1,Ready:m.phase==1,Cursor:game.MenuCursorState{Current:m.startCursor,Max:7}}
}
func (fakeFieldMenuDecoder) StartMenuEntryIndex(_ game.MemoryReader, entry game.StartMenuEntry)(int,bool){
	if entry==game.StartMenuItems { return 5,true }
	if entry==game.StartMenuPokemon { return 2,true }
	return 0,false
}

type fakeFieldListDecoder struct{}
func (fakeFieldListDecoder) DecodeListMenu(r game.MemoryReader) game.ListMenuState {
	m:=r.(*fakeFieldItemMachine)
	return game.ListMenuState{Visible:m.phase==2,Kind:game.ListMenuItems,Position:m.listPos}
}

type fakeFieldPartyDecoder struct{}
func (fakeFieldPartyDecoder) DecodePartyMenu(r game.MemoryReader) game.PartyMenuState {
	m:=r.(*fakeFieldItemMachine)
	return game.PartyMenuState{Visible:m.phase==4,Kind:game.PartyMenuItemUse,Cursor:game.MenuCursorState{Current:m.partyCursor,Max:len(m.party)-1}}
}

func TestGenericFieldItemUsesSemanticGen2Layout(t *testing.T) {
	m:=newFakeFieldItemMachine()
	err:=useFieldItemWithDecoders(m,fakeFieldMedicine,1,fakeFieldItemDecoder{},fakeFieldInventoryDecoder{},fakeFieldMenuDecoder{},fakeFieldListDecoder{},fakeFieldPartyDecoder{})
	if err!=nil { t.Fatalf("use field item: %v",err) }
	if got:=m.party[1].HP; got!=60 { t.Fatalf("HP=%d want 60",got) }
	if _,q:=fieldItemInventoryEntry((fakeFieldInventoryDecoder{}).DecodeInventory(m),fakeFieldMedicine);q!=1 { t.Fatalf("medicine qty=%d want 1",q) }
	if m.startCursor!=5 { t.Fatalf("Items cursor=%d want fake Gen-II index 5",m.startCursor) }
}

func TestGenericFieldItemSelectsSemanticPPMove(t *testing.T) {
	m:=newFakeFieldItemMachine()
	err:=useFieldItemWithDecoders(m,fakeFieldEther,1,fakeFieldItemDecoder{},fakeFieldInventoryDecoder{},fakeFieldMenuDecoder{},fakeFieldListDecoder{},fakeFieldPartyDecoder{})
	if err!=nil { t.Fatalf("use PP item: %v",err) }
	if got:=m.party[1].PP[1]; got!=5 { t.Fatalf("PP slot 1=%d want 5",got) }
	if m.moveCursor!=1 { t.Fatalf("move cursor=%d want exhausted move slot 1",m.moveCursor) }
}

func TestGenericRepelUsesProfileDurationAndWideID(t *testing.T) {
	m:=newFakeFieldItemMachine()
	err:=useRepelWithDecoders(m,fakeFieldRepel,fakeFieldItemDecoder{},fakeFieldInventoryDecoder{},fakeFieldMenuDecoder{},fakeFieldListDecoder{})
	if err!=nil { t.Fatalf("use repel: %v",err) }
	if m.repel!=321 { t.Fatalf("repel steps=%d want 321",m.repel) }
	if _,q:=fieldItemInventoryEntry((fakeFieldInventoryDecoder{}).DecodeInventory(m),fakeFieldRepel);q!=0 { t.Fatalf("repel qty=%d want 0",q) }
}
