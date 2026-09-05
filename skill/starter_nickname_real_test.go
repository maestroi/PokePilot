package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

func TestStarterKeepsSpeciesName(t *testing.T) {
	if testing.Short() {
		t.Skip("full opening story; not part of the -short gate")
	}
	m := fixture.Load(t, "reds_bedroom")
	if err := skill.GetStarter(m, m.ROM(), skill.StarterCharmander, skill.StatAwareMove(m.ROM())); err != nil {
		t.Fatalf("GetStarter: %v", err)
	}

	// pokered.sym: wPartyMon1Nick / wPartyMonNicks = 0xd2b5. Gen 1 stores
	// uppercase letters as 0x80..0x99 and terminates names with 0x50 ('@').
	const partyMon1Nick uint16 = 0xd2b5
	name := make([]byte, 0, 10)
	for i := uint16(0); i < 11; i++ {
		b := m.Peek8(partyMon1Nick + i)
		if b == 0x50 {
			break
		}
		if b < 0x80 || b > 0x99 {
			t.Fatalf("starter nickname contains unexpected encoded byte %#02x at offset %d", b, i)
		}
		name = append(name, 'A'+(b-0x80))
	}
	if got, want := string(name), "CHARMANDER"; got != want {
		t.Fatalf("starter nickname = %q, want default species name %q", got, want)
	}
}
