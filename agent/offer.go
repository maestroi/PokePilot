package agent

import (
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

// Knowledge is what a run has accumulated: the maps the player has stood on,
// the place names the game has actually shown — a visited map, or a name
// that appeared in decoded dialogue — and the objectives already completed.
// It grows during the run and is derived ONLY from what the game has shown.
// It is never seeded from the ROM's map table or from a list written out in
// advance: seeding it is the difference between an agent that explores and
// an agent that reads our notes.
//
// Adjacency is the one field that is not game-shown, and it is kept apart
// on purpose: it is route geometry (which map's doors lead where), built
// once by Run from the ROM, because "reachable from where you stand" is a
// question about exits and deterministic code keeps the geometry. It names
// no places; it only answers where the doors of a map lead.
type Knowledge struct {
	Visited   map[uint8]bool    // maps the player has stood on
	Places    map[string]bool   // place names the game has shown (visited or spoken)
	Completed map[string]bool   // objectives already completed, by Objective.String()
	Adjacency map[uint8][]uint8 // for each map id, the map ids its exits lead to
}

// NewKnowledge returns an empty Knowledge over the given route geometry.
// All maps start empty: a fresh run has seen nothing.
func NewKnowledge(adjacency map[uint8][]uint8) *Knowledge {
	return &Knowledge{
		Visited:   map[uint8]bool{},
		Places:    map[string]bool{},
		Completed: map[string]bool{},
		Adjacency: adjacency,
	}
}

// SawMap records that the player has stood on the map. A map the player has
// stood on is a place the player knows, whatever its name.
func (k *Knowledge) SawMap(id uint8) { k.Visited[id] = true }

// SawDialogue scans decoded dialogue lines for place names and adds any it
// finds. The place table is used as a VOCABULARY for the match — which
// names are recognizable — not as a seed: a name enters Knowledge only when
// the game actually said it. Matching is on word boundaries, so "Route 22"
// does not count as a mention of "route 2".
func (k *Knowledge) SawDialogue(lines []string) {
	for _, line := range lines {
		low := strings.ToLower(line)
		for _, name := range skill.PlaceNames() {
			if mentions(low, name) {
				k.Places[name] = true
			}
		}
	}
}

// Done records a completed objective. Only one-shot objectives are dropped
// from the menu because of this (see Offer): a completed heal is no reason
// to stop offering heals, and neither is a lost gym challenge — the run is
// meant to train, heal, and come back.
func (k *Knowledge) Done(o Objective) { k.Completed[o.String()] = true }

// mentions reports whether line contains name as a whole word: the match
// must not be embedded in a longer alphanumeric run on either side, so
// "route 2" is found in "take route 2 north" but not in "route 22".
func mentions(line, name string) bool {
	from := 0
	for {
		i := strings.Index(line[from:], name)
		if i < 0 {
			return false
		}
		i += from
		beforeOK := i == 0 || !isAlnum(line[i-1])
		end := i + len(name)
		afterOK := end == len(line) || !isAlnum(line[end])
		if beforeOK && afterOK {
			return true
		}
		from = i + 1
	}
}

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
}

// Offer builds the objectives that make sense HERE, NOW: the places the
// player could plausibly know — where they have stood, where the doors of
// where they stand lead, and names the game has spoken — plus the verbs
// whose preconditions currently hold. It is rebuilt every round from the
// current observation; a menu built once at startup offers the same
// everything-forever question on every frame, including places the player
// has no way of knowing exist.
//
// It says what is POSSIBLE, never what is WISE. An objective that is legal
// but unwise stays on the list: walking into the gym underlevelled is
// offered, and losing is the planner's mistake to make. The moment Offer
// starts withholding legal-but-unwise objectives, we are playing the game
// again. The safety property is unchanged — stronger, in fact: the planner
// still picks from a numbered list and can never invent an action, and the
// list now fits the situation instead of the whole ROM.
func Offer(obs Observation, known *Knowledge) []Objective {
	out := make([]Objective, 0, 8)

	// The starter is on the menu only while the player has no Pokemon:
	// the game offers no second one, and a satisfied starter is an
	// objective that succeeds instantly forever — the planner picks it,
	// nothing changes, and the run stalls having achieved nothing.
	if obs.PartyCount == 0 {
		out = append(out,
			Objective{Kind: KindStarter, Starter: skill.StarterCharmander},
			Objective{Kind: KindStarter, Starter: skill.StarterSquirtle},
			Objective{Kind: KindStarter, Starter: skill.StarterBulbasaur},
		)
	}

	// Places the player could plausibly know exist: maps already visited,
	// the maps the current map's exits lead to (one step out — the doors of
	// where you stand are what a player standing there can see; deeper
	// reachability is our map, not the player's), and places the game has
	// named in dialogue. The place table supplies the coordinates a known
	// name stands for; it does not put any name on this list by itself.
	knownMaps := map[uint8]bool{obs.Map: true}
	for m := range known.Visited {
		knownMaps[m] = true
	}
	for _, n := range known.Adjacency[obs.Map] {
		knownMaps[n] = true
	}
	for name := range known.Places {
		if d, ok := skill.Place(name); ok {
			knownMaps[d.Map] = true
		}
	}
	for _, name := range skill.PlaceNames() { // sorted: a stable menu order
		d, _ := skill.Place(name)
		if !knownMaps[d.Map] {
			continue
		}
		if d.Map == obs.Map && d.X == obs.X && d.Y == obs.Y {
			continue // already standing on it; "go" would be a no-op
		}
		out = append(out, Objective{Kind: KindGoTo, Place: name})
	}

	// Verbs, gated on preconditions that are facts about the situation —
	// never judgements about it.
	if !known.Completed[Objective{Kind: KindErrand}.String()] {
		out = append(out, Objective{Kind: KindErrand}) // one-shot story event
	}
	if hasBalls(obs) {
		if sp, ok := SpeciesByName("caterpie"); ok {
			out = append(out, Objective{Kind: KindCatch, Species: sp})
		}
	}
	if isCenter(obs.MapName) {
		out = append(out, Objective{Kind: KindHeal})
	}
	// The gym objective fights Brock in the Pewter Gym; elsewhere it has
	// no one to fight. That is a precondition of the verb, not a verdict
	// on whether the party is ready for it — underlevelled stays offered.
	if d, ok := skill.Place("pewter gym"); ok && d.Map == obs.Map {
		out = append(out, Objective{Kind: KindGym})
	}
	if obs.HasGrass {
		out = append(out, Objective{Kind: KindTrain, Level: 12})
	}
	if isMart(obs.MapName) {
		if it, ok := ItemByName("potion"); ok {
			out = append(out, Objective{Kind: KindBuy, Item: it, Qty: 3})
		}
	}

	// The people and items of this map, from the ROM map header (see
	// MapObjects): each person is a KindTalk at its tile, each item a
	// KindPickup with the item named. Trainers are REPORTED in MapObjects
	// but NOT offered — a fightable trainer verb is a separate slice, and
	// offering one with no verb behind it manufactures a guaranteed failed
	// objective every round. Collected items are not filtered: there is no
	// data source for it, and a vanished ball fails Pickup's bag
	// postcondition as an ordinary failed objective.
	for _, o := range obs.MapObjects {
		switch o.Kind {
		case "person":
			out = append(out, Objective{Kind: KindTalk, X: o.X, Y: o.Y})
		case "item":
			if id, ok := ItemByName(o.Item); ok {
				out = append(out, Objective{Kind: KindPickup, X: o.X, Y: o.Y, Item: id})
			}
		}
	}
	return out
}

// hasBalls says the bag holds at least one usable ball. The bag is decoded
// and named by Observe; a zero-quantity entry never reaches it.
func hasBalls(obs Observation) bool {
	for _, it := range obs.Bag {
		if it.Name == "pokeball" || it.Name == "great ball" {
			return true
		}
	}
	return false
}

// isCenter and isMart read the map's decomp name, which carries the
// building type by convention: every center is *_POKECENTER, every mart
// *_MART. A fact about the map the player is standing in, like HasGrass.
func isCenter(mapName string) bool {
	return strings.Contains(strings.ToUpper(mapName), "POKECENTER")
}

func isMart(mapName string) bool {
	return strings.Contains(strings.ToUpper(mapName), "MART")
}
