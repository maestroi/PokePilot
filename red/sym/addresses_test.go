package sym

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

const symFile = "/home/maestro/.cache/pokered/pokered.sym"

func loadSym(t *testing.T) map[string]uint16 {
	t.Helper()
	data, err := os.ReadFile(symFile)
	if err != nil {
		t.Skip("pokered.sym not available: ", err)
	}
	sym := make(map[string]uint16)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		parts := strings.SplitN(fields[0], ":", 2)
		if len(parts) != 2 {
			continue
		}
		addr, err := strconv.ParseUint(parts[1], 16, 16)
		if err != nil {
			continue
		}
		sym[fields[1]] = uint16(addr)
	}
	return sym
}

func TestAddressesMatchSymbolFile(t *testing.T) {
	sym := loadSym(t)

	pairs := []struct {
		label    string
		constant uint16
	}{
		{"wCurMap", CurMap},
		{"wYCoord", YCoord},
		{"wXCoord", XCoord},
		{"wYBlockCoord", YBlockCoord},
		{"wXBlockCoord", XBlockCoord},
		{"wCurMapTileset", CurMapTileset},
		{"wCurMapHeight", CurMapHeight},
		{"wCurMapWidth", CurMapWidth},
		{"wPlayerMovingDirection", PlayerMovingDirection},
		{"wPlayerDirection", PlayerDirection},
		{"wWalkCounter", WalkCounter},
		{"wPlayerName", PlayerName},
		{"wPartyCount", PartyCount},
		{"wPartySpecies", PartySpecies},
		{"wPartyMon1", PartyMon1},
		{"wNumBagItems", NumBagItems},
		{"wBagItems", BagItems},
		{"wPlayerMoney", PlayerMoney},
		{"wObtainedBadges", ObtainedBadges},
		{"wEventFlags", EventFlags},
		{"wIsInBattle", IsInBattle},
		{"wBattleResult", BattleResult},
		{"wEnemyMonSpecies", EnemyMonSpecies},
		{"wEnemyMonLevel", EnemyMonLevel},
		{"wEnemyMonMaxHP", EnemyMonMaxHP},
		{"wBattleMonSpecies", BattleMonSpecies},
		{"wBattleMonHP", BattleMonHP},
		{"wBattleMonMoves", BattleMonMoves},
		{"wBattleMonLevel", BattleMonLevel},
		{"wBattleMonMaxHP", BattleMonMaxHP},
		{"wBattleMonPP", BattleMonPP},
		{"wPartyMon1HP", PartyMon1HP},
		{"wCurrentMenuItem", CurrentMenuItem},
		{"wMaxMenuItem", MaxMenuItem},
		{"wTextBoxID", TextBoxID},
		{"wFontLoaded", FontLoaded},
		{"wJoyIgnore", JoyIgnore},
		{"hJoyHeld", JoyHeld},
		{"hJoyPressed", JoyPressed},
		{"hJoyInput", JoyInput},
	}
	for _, p := range pairs {
		want, ok := sym[p.label]
		if !ok {
			t.Errorf("label %s not found in %s", p.label, symFile)
			continue
		}
		if uint16(p.constant) != want {
			t.Errorf("%s = 0x%04X, want 0x%04X (from %s)", p.label, p.constant, want, symFile)
		}
	}

	spriteBase, ok := sym["wSpritePlayerStateData1"]
	if !ok {
		t.Errorf("label wSpritePlayerStateData1 not found in %s", symFile)
	} else if SpritePlayerFacing != spriteBase+9 {
		t.Errorf("SpritePlayerFacing = 0x%04X, want 0x%04X (wSpritePlayerStateData1 + 9)", SpritePlayerFacing, spriteBase+9)
	}

	mon1, ok := sym["wPartyMon1"]
	if !ok {
		t.Fatal("wPartyMon1 not found in symbol file")
	}
	mon2, ok := sym["wPartyMon2"]
	if !ok {
		t.Fatal("wPartyMon2 not found in symbol file")
	}
	if PartyMonSize != mon2-mon1 {
		t.Errorf("PartyMonSize = 0x%X, want 0x%X (wPartyMon2 - wPartyMon1)", PartyMonSize, mon2-mon1)
	}

	offsets := []struct {
		label    string
		constant uint16
	}{
		{"wPartyMon1HP", MonHP},
		{"wPartyMon1Status", MonStatus},
		{"wPartyMon1Moves", MonMoves},
		{"wPartyMon1PP", MonPP},
		{"wPartyMon1Level", MonLevel},
		{"wPartyMon1MaxHP", MonMaxHP},
	}
	for _, o := range offsets {
		want, ok := sym[o.label]
		if !ok {
			t.Errorf("label %s not found in %s", o.label, symFile)
			continue
		}
		if o.constant != want-mon1 {
			t.Errorf("%s offset = 0x%X, want 0x%X (0x%04X - wPartyMon1)", o.label, o.constant, want-mon1, want)
		}
	}
}
