package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// gen1MatchupMon decodes the matchup-relevant facts of one party member.
func gen1MatchupMon(romData []byte, mon state.Mon) PartyMon {
	out := PartyMon{Level: mon.Level, HP: mon.HP, MaxHP: mon.MaxHP}
	out.Types = append(out.Types, redTypeName(mon.Type1))
	if mon.Type2 != mon.Type1 {
		out.Types = append(out.Types, redTypeName(mon.Type2))
	}
	for slot, id := range mon.Moves {
		if id == 0 {
			continue
		}
		if mv, err := rom.LookupMove(romData, id); err == nil {
			out.Moves = append(out.Moves, PartyMove{Move: Move{Power: mv.Power, Type: redTypeName(mv.Type)}, PP: mon.PP[slot]})
		}
	}
	return out
}

// prepareChallengeLead leads with the strongest level-ready counter before a
// ROM-profiled challenge when the current lead is not itself a counter. The
// fight's own switch policy still runs; this only stops a resisted lead (an
// Electric lead into Giovanni) from opening the battle.
func prepareChallengeLead(m *emu.Emu, romData []byte, o Objective) error {
	if o.Kind != KindGym && o.Kind != KindProgress {
		return nil
	}
	profile := challengeProfileIn(gen1ChallengeProfiles(romData), o)
	if !profile.Matchup.Known() || len(profile.Matchup.Preferred) == 0 {
		return nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) || state.DecodeBattle(&mem) != nil {
		return nil
	}
	party := state.DecodeParty(&mem)
	obs := Observation{}
	for _, mon := range party.Mons {
		obs.Party = append(obs.Party, gen1MatchupMon(romData, mon))
	}
	counters := partyCounters(obs, profile.Matchup)
	if len(counters) == 0 || counters[0].Slot == 0 || counters[0].Level < counterLevelTarget(profile.Matchup) {
		return nil
	}
	for _, c := range counters {
		if c.Slot == 0 {
			return nil // the lead already carries a preferred attack
		}
	}
	if err := skill.SetLead(m, counters[0].Slot); err != nil {
		return fmt.Errorf("agent: %s: lead counter slot %d: %w", o, counters[0].Slot, err)
	}
	return nil
}
