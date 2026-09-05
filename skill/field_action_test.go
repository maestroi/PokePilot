package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func fieldTestMem(move FieldMove, badge bool, learned bool, hmOwned bool) *state.Mem {
	m := new(state.Mem)
	spec, _ := FieldMoveSpecFor(move)
	if badge {
		m[sym.ObtainedBadges] = 1 << uint8(spec.Badge)
	}
	if learned {
		m[sym.PartyCount] = 1
		m[sym.PartyMon1+sym.MonMoves] = spec.MoveID
	}
	if hmOwned {
		m[sym.NumBagItems] = 1
		m[sym.BagItems] = spec.HMItem
		m[sym.BagItems+1] = 1
	}
	return m
}

func makeFieldControllable(m *state.Mem) {
	m[sym.CurMapWidth] = 1
	m[sym.CurMapHeight] = 1
	m[sym.FontLoaded] = 0
	m[sym.JoyIgnore] = 0
	m[sym.WalkCounter] = 0
}

func TestFieldMoveSpecsMatchPokemonRed(t *testing.T) {
	tests := []struct {
		move   FieldMove
		name   string
		hm     uint8
		moveID uint8
		menuID uint8
		badge  state.Badge
		kind   FieldActionKind
	}{
		{FieldCut, "CUT", 0xC4, 0x0F, 1, state.BadgeCascade, FieldActionTargeted},
		{FieldFly, "FLY", 0xC5, 0x13, 2, state.BadgeThunder, FieldActionTransition},
		{FieldSurf, "SURF", 0xC6, 0x39, 4, state.BadgeSoul, FieldActionMode},
		{FieldStrength, "STRENGTH", 0xC7, 0x46, 5, state.BadgeRainbow, FieldActionTargeted},
		{FieldFlash, "FLASH", 0xC8, 0x94, 6, state.BadgeBoulder, FieldActionMode},
	}
	for _, tt := range tests {
		spec, ok := FieldMoveSpecFor(tt.move)
		if !ok {
			t.Fatalf("FieldMoveSpecFor(%v) not found", tt.move)
		}
		if spec.Name != tt.name || spec.HMItem != tt.hm || spec.MoveID != tt.moveID || spec.MenuID != tt.menuID || spec.Badge != tt.badge || spec.Kind != tt.kind {
			t.Errorf("spec %v = %+v, want name=%s hm=%#02x move=%#02x menu=%d badge=%v kind=%d",
				tt.move, spec, tt.name, tt.hm, tt.moveID, tt.menuID, tt.badge, tt.kind)
		}
	}
}

func TestFieldCapabilityHMOwnershipIsNotUsability(t *testing.T) {
	m := fieldTestMem(FieldSurf, true, false, true)
	cap := FieldCapabilityFor(m, FieldSurf)
	if !cap.BadgeOwned || !cap.HMOwned {
		t.Fatalf("preconditions not decoded: %+v", cap)
	}
	if cap.Learned || cap.Usable || cap.PartySlot >= 0 {
		t.Fatalf("HM03 ownership without learned Surf became a capability: %+v", cap)
	}
}

func TestFieldCapabilityRequiresBadgeAndLearnedMove(t *testing.T) {
	withoutBadge := FieldCapabilityFor(fieldTestMem(FieldStrength, false, true, false), FieldStrength)
	if !withoutBadge.Learned || withoutBadge.Usable {
		t.Fatalf("learned Strength without Rainbow Badge = %+v, want learned but unusable", withoutBadge)
	}

	usable := FieldCapabilityFor(fieldTestMem(FieldStrength, true, true, false), FieldStrength)
	if !usable.Learned || !usable.BadgeOwned || !usable.Usable || usable.PartySlot != 0 {
		t.Fatalf("badge + learned Strength = %+v, want usable in slot 0", usable)
	}
}

func TestFieldCapabilitiesStableForPartyPlanning(t *testing.T) {
	m := fieldTestMem(FieldCut, true, true, false)
	caps := FieldCapabilities(m)
	moves := ProgressionFieldMoves()
	if len(caps) != len(moves) || len(caps) != 5 {
		t.Fatalf("capabilities=%d progression moves=%d, want 5", len(caps), len(moves))
	}
	for i, move := range moves {
		if caps[i].Move != move {
			t.Fatalf("capability[%d].Move=%v, want %v", i, caps[i].Move, move)
		}
	}
	if !caps[0].Usable || caps[0].Move != FieldCut {
		t.Fatalf("Cut capability = %+v, want usable", caps[0])
	}
}

func TestCutRouteCapabilityUsesSharedFieldCapability(t *testing.T) {
	m := fieldTestMem(FieldCut, true, true, false)
	if !cutCapabilityRecoverable(nil, m) {
		t.Fatal("route recovery rejected a learned, badged Cut capability")
	}

	m = fieldTestMem(FieldCut, false, true, true)
	if cutCapabilityRecoverable(nil, m) {
		t.Fatal("route recovery accepted Cut without the Cascade Badge")
	}
}

func TestBoulderAheadUsesLiveSpriteContext(t *testing.T) {
	m := new(state.Mem)
	m[sym.CurMap] = 1
	m[sym.XCoord] = 10
	m[sym.YCoord] = 10
	m[sym.SpritePlayerFacing] = byte(state.FacingRight)

	// Sprite slot 1: data1 picture id/image index plus data2 biased map X/Y.
	m[sym.SpritePlayerStateData1+0x10] = fieldBoulderPictureID
	m[sym.SpritePlayerStateData1+0x12] = 0
	m[sym.SpriteStateData2+0x10+0x04] = 14 // y 10 + bias 4
	m[sym.SpriteStateData2+0x10+0x05] = 15 // x 11 + bias 4
	if !boulderAhead(m) {
		t.Fatal("boulder directly in front was not detected")
	}

	m[sym.SpriteStateData2+0x10+0x05] = 16
	if boulderAhead(m) {
		t.Fatal("non-adjacent boulder was treated as the Strength target")
	}
}

func TestFieldActionCompletionUsesROMState(t *testing.T) {
	m := new(state.Mem)
	makeFieldControllable(m)

	cut, _ := FieldMoveSpecFor(FieldCut)
	m[sym.ActionResult] = 1
	if !fieldActionComplete(m, cut) {
		t.Fatal("Cut action result was not accepted")
	}

	surf, _ := FieldMoveSpecFor(FieldSurf)
	if fieldActionComplete(m, surf) {
		t.Fatal("Surf completed without entering surfing state")
	}
	m[sym.WalkBikeSurfState] = fieldSurfingState
	if !fieldActionComplete(m, surf) {
		t.Fatal("Surf action result + surfing state was not accepted")
	}

	strength, _ := FieldMoveSpecFor(FieldStrength)
	m[sym.ActionResult] = 0 // Strength completion is its dedicated live flag.
	if fieldActionComplete(m, strength) {
		t.Fatal("Strength completed before BIT_STRENGTH_ACTIVE was set")
	}
	m[sym.StatusFlags1] = fieldStrengthActiveBit
	if !fieldActionComplete(m, strength) {
		t.Fatal("Strength active flag was not accepted")
	}
}
