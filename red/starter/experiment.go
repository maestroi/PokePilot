// Package starter implements reproducible Pokemon Red starter experiments.
// It owns experiment selection and the tiny ROM patch; planners only observe
// the resulting party and never need randomizer-specific behavior.
package starter

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/game"
	reddata "github.com/maestroi/pokepilot/red/data"
	redrom "github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/sym"
)

type Mode string

type Pool string

const (
	ModeVanilla Mode = "vanilla"
	ModeRandom  Mode = "random"
	ModeFixed   Mode = "fixed"

	PoolAny        Pool = "any"
	PoolBasic      Pool = "basic"
	PoolReasonable Pool = "reasonable"
)

// Selection is the fully resolved starter experiment for one run. Custom and
// random experiments intentionally use Oak's middle (Squirtle) ball so the
// physical script path is constant while only the received species varies.
type Selection struct {
	Request string
	Mode    Mode
	Pool    Pool
	Species game.SpeciesID
	Raw     uint8
	Slot    string
	Seed    int64
}

func (s Selection) Experiment() bool { return s.Mode == ModeRandom || s.Mode == ModeFixed }

func (s Selection) Summary() string {
	if s.Species == "" {
		return string(s.Mode)
	}
	if s.Mode == ModeRandom {
		return fmt.Sprintf("random:%s -> %s (seed %d)", s.Pool, s.Species, s.Seed)
	}
	return fmt.Sprintf("%s -> %s", s.Mode, s.Species)
}

// Resolve accepts the existing canonical starter names plus experiment forms:
// random[:reasonable|basic|any], fixed:<species>, or a bare Gen I species
// such as mew or mewtwo. Empty remains the historical "let the LLM decide".
func Resolve(request string, seed int64) (Selection, error) {
	req := strings.ToLower(strings.TrimSpace(request))
	if req == "" {
		return Selection{Request: request, Mode: ModeVanilla, Seed: seed}, nil
	}
	if req == "charmander" || req == "squirtle" || req == "bulbasaur" {
		raw, _ := reddata.SpeciesRaw(game.SpeciesID(req))
		return Selection{Request: request, Mode: ModeVanilla, Species: game.SpeciesID(req), Raw: raw, Slot: req, Seed: seed}, nil
	}
	if req == "random" || strings.HasPrefix(req, "random:") {
		pool := PoolReasonable
		if strings.HasPrefix(req, "random:") {
			pool = Pool(strings.TrimSpace(strings.TrimPrefix(req, "random:")))
		}
		members, err := poolMembers(pool)
		if err != nil {
			return Selection{}, err
		}
		chosen := deterministicChoice(seed, pool, members)
		raw, ok := reddata.SpeciesRaw(chosen)
		if !ok {
			return Selection{}, fmt.Errorf("starter: internal pool species %q is not a Red species", chosen)
		}
		return Selection{Request: request, Mode: ModeRandom, Pool: pool, Species: chosen, Raw: raw, Slot: "squirtle", Seed: seed}, nil
	}

	name := req
	if strings.HasPrefix(name, "fixed:") {
		name = strings.TrimSpace(strings.TrimPrefix(name, "fixed:"))
	}
	name = normalizeSpeciesName(name)
	raw, ok := reddata.SpeciesRaw(game.SpeciesID(name))
	if !ok {
		return Selection{}, fmt.Errorf("starter: unknown Pokemon %q; use a Gen I species or random:reasonable, random:basic, random:any", request)
	}
	return Selection{Request: request, Mode: ModeFixed, Species: game.SpeciesID(name), Raw: raw, Slot: "squirtle", Seed: seed}, nil
}

func normalizeSpeciesName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "nidoran-f", "nidoran-female", "nidoran♀":
		return "nidoran♀"
	case "nidoran-m", "nidoran-male", "nidoran♂":
		return "nidoran♂"
	case "mr-mime", "mr mime", "mrmime", "mr.mime":
		return "mr.mime"
	case "farfetchd", "farfetch-d", "farfetch’d", "farfetch'd":
		return "farfetch'd"
	default:
		return name
	}
}

func deterministicChoice(seed int64, pool Pool, members []game.SpeciesID) game.SpeciesID {
	key := sym.ROMSHA1 + "\x00" + strconv.FormatInt(seed, 10) + "\x00" + string(pool)
	sum := sha256.Sum256([]byte(key))
	n := binary.LittleEndian.Uint64(sum[:8])
	return members[n%uint64(len(members))]
}

func poolMembers(pool Pool) ([]game.SpeciesID, error) {
	switch pool {
	case PoolAny:
		return reddata.SpeciesIDs(), nil
	case PoolBasic:
		return speciesIDs(basicSpecies), nil
	case PoolReasonable:
		return speciesIDs(reasonableSpecies), nil
	default:
		return nil, fmt.Errorf("starter: unknown random pool %q: want reasonable, basic, or any", pool)
	}
}

func speciesIDs(names []string) []game.SpeciesID {
	out := make([]game.SpeciesID, len(names))
	for i, name := range names {
		out[i] = game.SpeciesID(name)
	}
	return out
}

// basicSpecies is the Gen I base/unevolved set, including standalone species.
// The deliberately broad definition makes legendary/rare experiments possible;
// the reasonable pool below is the safer default for unattended runs.
var basicSpecies = []string{
	"bulbasaur", "charmander", "squirtle", "caterpie", "weedle", "pidgey", "rattata", "spearow",
	"ekans", "pikachu", "sandshrew", "nidoran♀", "nidoran♂", "clefairy", "vulpix", "jigglypuff",
	"zubat", "oddish", "paras", "venonat", "diglett", "meowth", "psyduck", "mankey", "growlithe",
	"poliwag", "abra", "machop", "bellsprout", "tentacool", "geodude", "ponyta", "slowpoke",
	"magnemite", "farfetch'd", "doduo", "seel", "grimer", "shellder", "gastly", "onix", "drowzee",
	"krabby", "voltorb", "exeggcute", "cubone", "hitmonlee", "hitmonchan", "lickitung", "koffing",
	"rhyhorn", "chansey", "tangela", "kangaskhan", "horsea", "goldeen", "staryu", "mr.mime", "scyther",
	"jynx", "electabuzz", "magmar", "pinsir", "tauros", "magikarp", "lapras", "ditto", "eevee",
	"porygon", "omanyte", "kabuto", "aerodactyl", "snorlax", "articuno", "zapdos", "moltres", "dratini",
	"mewtwo", "mew",
}

// reasonableSpecies is intentionally experiment configuration, not planner
// behavior. It keeps ordinary early-game-capable base species while excluding
// no-damage openings (Abra/Magikarp), transform-only Ditto, and deliberately
// exceptional fossil/static/legendary power spikes.
var reasonableSpecies = []string{
	"bulbasaur", "charmander", "squirtle", "caterpie", "weedle", "pidgey", "rattata", "spearow",
	"ekans", "pikachu", "sandshrew", "nidoran♀", "nidoran♂", "clefairy", "vulpix", "jigglypuff",
	"zubat", "oddish", "paras", "venonat", "diglett", "meowth", "psyduck", "mankey", "growlithe",
	"poliwag", "machop", "bellsprout", "tentacool", "geodude", "ponyta", "slowpoke", "magnemite",
	"doduo", "seel", "grimer", "shellder", "gastly", "onix", "drowzee", "krabby", "voltorb",
	"exeggcute", "cubone", "koffing", "rhyhorn", "horsea", "goldeen", "staryu", "eevee", "dratini",
}

// PatchInfo records exactly how the derived ROM differs from the supported
// base image. Offsets are absolute file offsets and Changes contains only bytes
// whose values actually changed.
type PatchInfo struct {
	BaseSHA1      string
	EffectiveSHA1 string
	Description   string
	Changes       []Change
}

type Change struct {
	Offset int
	From   byte
	To     byte
}

var middleBallSignature = []byte{
	0x3e, 0x99, // ld a, BULBASAUR (Blue's starter temp)
	0xea, 0x3d, 0xcd, // ld [wRivalStarterTemp], a
	0x3e, 0x04, // ld a, OAKSLAB_BULBASAUR_POKE_BALL
	0xea, 0x3e, 0xcd, // ld [wRivalStarterBallSpriteIndex], a
	0x3e, 0xb1, // ld a, SQUIRTLE (player species: patched byte is +11)
	0x06, 0x03, // ld b, OAKSLAB_SQUIRTLE_POKE_BALL
	0x18, // jr OaksLabSelectedPokeBallScript
}

// Patch derives an in-memory experiment ROM. Vanilla selections return an
// unmodified copy. Custom/random selections alter exactly the middle-ball
// species immediate plus Oak's middle-slot comparison; Blue's starter temp,
// trainer team, maps, encounters, items and all other scripts remain untouched.
func Patch(base []byte, selection Selection) ([]byte, PatchInfo, error) {
	if err := redrom.Verify(base); err != nil {
		return nil, PatchInfo{}, fmt.Errorf("starter: base ROM: %w", err)
	}
	out := append([]byte(nil), base...)
	info := PatchInfo{BaseSHA1: redrom.SHA1Hex(base)}
	if !selection.Experiment() {
		info.EffectiveSHA1 = info.BaseSHA1
		info.Description = "vanilla ROM"
		return out, info, nil
	}

	ball, err := findUnique(out, middleBallSignature)
	if err != nil {
		return nil, PatchInfo{}, fmt.Errorf("starter: locate Oak middle-ball script: %w", err)
	}
	if err := patchByte(out, ball+11, selection.Raw, &info); err != nil {
		return nil, PatchInfo{}, err
	}

	compare, err := findUniqueMiddleSlotCompare(out)
	if err != nil {
		return nil, PatchInfo{}, fmt.Errorf("starter: locate Oak chosen-starter compare: %w", err)
	}
	if err := patchByte(out, compare+8, selection.Raw, &info); err != nil {
		return nil, PatchInfo{}, err
	}

	info.EffectiveSHA1 = redrom.SHA1Hex(out)
	info.Description = fmt.Sprintf("Oak middle ball: Squirtle -> %s; preserve Blue's vanilla starter/team", selection.Species)
	return out, info, nil
}

func patchByte(rom []byte, offset int, to byte, info *PatchInfo) error {
	if offset < 0 || offset >= len(rom) {
		return fmt.Errorf("starter: patch offset %#x outside ROM", offset)
	}
	from := rom[offset]
	rom[offset] = to
	if from != to {
		info.Changes = append(info.Changes, Change{Offset: offset, From: from, To: to})
	}
	return nil
}

func findUnique(data, signature []byte) (int, error) {
	first := bytes.Index(data, signature)
	if first < 0 {
		return 0, fmt.Errorf("signature not found")
	}
	if bytes.Index(data[first+1:], signature) >= 0 {
		return 0, fmt.Errorf("signature is not unique")
	}
	return first, nil
}

func findUniqueMiddleSlotCompare(rom []byte) (int, error) {
	found := -1
	for i := 0; i+12 < len(rom); i++ {
		// ld a,[wPlayerStarter]; cp CHARMANDER; jr z,*; cp SQUIRTLE;
		// jr z,*; jr *  -- only branch offsets are intentionally ignored.
		if rom[i] != 0xfa || rom[i+1] != 0x17 || rom[i+2] != 0xd7 ||
			rom[i+3] != 0xfe || rom[i+4] != 0xb0 || rom[i+5] != 0x28 ||
			rom[i+7] != 0xfe || rom[i+8] != 0xb1 || rom[i+9] != 0x28 || rom[i+11] != 0x18 {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("signature is not unique")
		}
		found = i
	}
	if found < 0 {
		return 0, fmt.Errorf("signature not found")
	}
	return found, nil
}
