package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/rom"
)

// MoveLearningDecision is the deterministic result of comparing a level-up
// move against a full four-move set. ReplaceSlot is -1 when the move should be
// declined (or when an empty slot means no replacement is necessary).
//
// The type is intentionally generic enough for #46's TM/HM teaching path to
// reuse the same replacement reasoning instead of growing a second policy.
type MoveLearningDecision struct {
	Learn       bool
	ReplaceSlot int
	Offered     uint8
	BeforeScore int
	AfterScore  int
	Reason      string
}

const minMoveLearningImprovement = 4

// DecideNaturalMove decides whether a naturally offered move improves the
// current set and, when all four slots are occupied, which move to replace.
// It scores the whole set rather than raw base power so STAB, accuracy, PP,
// damaging coverage, useful status roles, and redundant moves all matter.
// Gen 1 HM moves are locked: the ROM itself refuses to forget them and the
// policy also treats that permanence as a strategic constraint up front.
func DecideNaturalMove(romData []byte, type1, type2 uint8, current [4]uint8, offered uint8) MoveLearningDecision {
	return decideNaturalMove(romData, type1, type2, current, offered, nil)
}

// decideNaturalMove is the battle-menu form. blocked contains move ids the ROM
// has already rejected as non-forgettable; normally HM detection means it is
// empty, but keeping the observed rejection as a backstop makes menu recovery
// deterministic even if another permanent move exists in a variant ROM.
func decideNaturalMove(romData []byte, type1, type2 uint8, current [4]uint8, offered uint8, blocked map[uint8]bool) MoveLearningDecision {
	d := MoveLearningDecision{ReplaceSlot: -1, Offered: offered}
	if offered == 0 {
		d.Reason = "decline empty offered move"
		return d
	}
	if _, err := rom.LookupMove(romData, offered); err != nil {
		d.Reason = fmt.Sprintf("decline undecodable offered move %d: %v", offered, err)
		return d
	}
	for _, id := range current {
		if id == offered {
			d.Reason = fmt.Sprintf("decline move %d: already known", offered)
			return d
		}
	}

	d.BeforeScore = moveSetScore(romData, type1, type2, current)

	// LearnMove does not ask a yes/no question when a slot is empty, but the
	// generic decision still models that state for tests and future TM/HM use.
	for slot, id := range current {
		if id != 0 {
			continue
		}
		next := current
		next[slot] = offered
		d.Learn = true
		d.AfterScore = moveSetScore(romData, type1, type2, next)
		d.Reason = fmt.Sprintf("learn move %d into empty slot %d", offered, slot)
		return d
	}

	offeredMove, _ := rom.LookupMove(romData, offered)
	offeredDamages := moveDealsDamage(offeredMove)
	currentDamagers := 0
	hasReplaceableStatus := false
	for _, id := range current {
		mv, err := rom.LookupMove(romData, id)
		if err != nil {
			// Unknown existing moves are preserved by scoring and are not used
			// to weaken the hard damaging-move safety rule.
			continue
		}
		if moveDealsDamage(mv) {
			currentDamagers++
		} else if !isGen1HM(id) && !blocked[id] {
			hasReplaceableStatus = true
		}
	}

	bestScore := d.BeforeScore
	bestSlot := -1
	for slot, id := range current {
		if id == 0 || isGen1HM(id) || blocked[id] {
			continue
		}
		mv, err := rom.LookupMove(romData, id)
		if err != nil {
			continue // never delete a move whose semantics we cannot prove
		}
		// The live Ivysaur failure that motivated #50 was exactly this shape:
		// Tackle + Vine Whip + two status moves, then PoisonPowder arrived and
		// slot-zero replacement discarded Tackle. If a status slot is
		// available, a new status move may not reduce two attacks to one.
		if !offeredDamages && currentDamagers >= 2 && moveDealsDamage(mv) && hasReplaceableStatus {
			continue
		}
		next := current
		next[slot] = offered
		score := moveSetScore(romData, type1, type2, next)
		if score > bestScore {
			bestScore, bestSlot = score, slot
		}
	}

	if bestSlot < 0 || bestScore-d.BeforeScore < minMoveLearningImprovement {
		d.AfterScore = d.BeforeScore
		d.Reason = fmt.Sprintf("decline move %d: no material move-set improvement (score %d)", offered, d.BeforeScore)
		return d
	}

	d.Learn = true
	d.ReplaceSlot = bestSlot
	d.AfterScore = bestScore
	d.Reason = fmt.Sprintf("learn move %d: score %d->%d replacing slot %d move %d",
		offered, d.BeforeScore, bestScore, bestSlot, current[bestSlot])
	return d
}

// moveSetScore values a four-move set in opponent-independent units. Battle
// choice still uses red/combat's live matchup model; this score answers the
// different question "which tools should this Pokémon keep for future
// battles?". Unique damaging types are rewarded, duplicates are discounted,
// and a set with no (or only one) damaging option is strongly penalized.
func moveSetScore(romData []byte, type1, type2 uint8, moves [4]uint8) int {
	score := 0
	damaging := 0
	damageTypes := map[uint8]int{}
	statusRoles := map[uint8]int{}

	for _, id := range moves {
		if id == 0 {
			continue
		}
		mv, err := rom.LookupMove(romData, id)
		if err != nil {
			// Existing unknown moves are expensive to throw away. Real Red ROM
			// moves always decode; this is a conservative variant-ROM fallback.
			score += 80
			continue
		}
		score += moveLearningQuality(mv, type1, type2)
		if moveDealsDamage(mv) {
			damaging++
			damageTypes[mv.Type]++
			if damageTypes[mv.Type] == 1 {
				score += 12
			} else {
				score -= 6 * (damageTypes[mv.Type] - 1)
			}
		} else {
			role := statusRole(mv.Effect)
			if role != 0 {
				statusRoles[role]++
				if statusRoles[role] > 1 {
					score -= 6
				}
			}
		}
	}

	switch damaging {
	case 0:
		score -= 200
	case 1:
		score -= 35
	default:
		score += 5 * damaging
	}
	return score
}

func moveLearningQuality(mv rom.Move, type1, type2 uint8) int {
	if moveDealsDamage(mv) {
		power := int(mv.Power)
		if power == 0 {
			switch mv.Effect {
			case rom.SpecialDamageEffect:
				power = 45
			case rom.SuperFangEffect:
				power = 55
			case rom.OHKOEffect:
				power = 40
			default:
				power = 30
			}
		}
		acc := int(mv.Accuracy)
		if acc == 0 {
			acc = 255
		}
		quality := power * acc / 255
		if mv.Type == type1 || mv.Type == type2 {
			quality = quality * 3 / 2
		}
		quality += minInt(int(mv.PP), 40) / 4
		// Two-turn and recoil moves remain useful, but their headline base
		// power overstates the future-set value compared with a one-turn move.
		switch mv.Effect {
		case 0x27, 0x2b: // CHARGE_EFFECT, FLY_EFFECT
			quality -= 8
		case 0x30: // RECOIL_EFFECT
			quality -= 3
		}
		if quality < 1 {
			quality = 1
		}
		return quality
	}

	base := statusMoveBase(mv.Effect)
	if base == 0 {
		return 2
	}
	acc := int(mv.Accuracy)
	if acc == 0 {
		acc = 255 // self-target/status moves can use the table's zero specially
	}
	quality := base * acc / 255
	quality += minInt(int(mv.PP), 40) / 10
	return quality
}

// moveDealsDamage includes Red's formula-bypassing damage effects. Using only
// Move.Power would classify Seismic Toss, Dragon Rage, Super Fang, etc. as
// status moves and could throw away a Pokémon's only real attack.
func moveDealsDamage(mv rom.Move) bool {
	return mv.Power > 0 || mv.Effect == rom.SpecialDamageEffect || mv.Effect == rom.SuperFangEffect || mv.Effect == rom.OHKOEffect
}

// statusMoveBase groups effects by practical role. Exact live-battle value is
// matchup-dependent; these deliberately coarse values are only for deciding
// which four tools are worth keeping. Strong control/recovery outranks simple
// one-stage debuffs, while inert or notoriously poor Gen 1 effects stay low.
func statusMoveBase(effect uint8) int {
	switch effect {
	case 0x20: // SLEEP_EFFECT
		return 48
	case 0x38: // HEAL_EFFECT
		return 44
	case 0x43: // PARALYZE_EFFECT
		return 40
	case 0x54: // LEECH_SEED_EFFECT
		return 36
	case 0x31: // CONFUSION_EFFECT
		return 32
	case 0x42: // POISON_EFFECT
		return 30
	case 0x40, 0x41, 0x4f: // LIGHT_SCREEN, REFLECT, SUBSTITUTE
		return 30
	case 0x32, 0x33, 0x34, 0x35, 0x36, 0x37: // two-stage self buffs
		return 28
	case 0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f: // two-stage target debuffs
		return 26
	case 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f: // one-stage self buffs
		return 20
	case 0x12, 0x13, 0x14, 0x15, 0x16, 0x17: // one-stage target debuffs
		return 16
	case 0x56: // DISABLE_EFFECT
		return 18
	case 0x19, 0x2e: // HAZE, MIST
		return 14
	case 0x52, 0x53: // MIMIC, METRONOME
		return 12
	case 0x2f: // FOCUS_ENERGY_EFFECT (broken in Gen 1; lowers crit chance)
		return 1
	case 0x55: // SPLASH_EFFECT
		return 0
	default:
		return 10
	}
}

func statusRole(effect uint8) uint8 {
	switch effect {
	case 0x20:
		return 1 // sleep
	case 0x42:
		return 2 // poison
	case 0x43:
		return 3 // paralysis
	case 0x31:
		return 4 // confusion
	case 0x54:
		return 5 // leech seed
	case 0x38:
		return 6 // recovery
	case 0x40, 0x41:
		return 7 // screens
	case 0x0a, 0x0b, 0x0c, 0x0d, 0x32, 0x33, 0x34, 0x35:
		return 8 // stat setup
	case 0x12, 0x13, 0x14, 0x15, 0x3a, 0x3b, 0x3c, 0x3d:
		return 9 // stat debuff
	default:
		return effect
	}
}

// HMs are permanent in Pokémon Red: the normal forget-move flow rejects them.
// Keep this explicit until #46 replaces it with the generic ROM-derived HM
// mapping used by TM/HM teaching.
func isGen1HM(moveID uint8) bool {
	switch moveID {
	case 15, 19, 57, 70, 148: // CUT, FLY, SURF, STRENGTH, FLASH
		return true
	default:
		return false
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
