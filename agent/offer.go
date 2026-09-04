package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// Knowledge is what a run has accumulated: the maps the player has stood on,
// the place names the game has actually shown — a visited map, or a name
// that appeared in decoded dialogue — the raw sentences the game has said
// that state a requirement or a blocked way, and the objectives already
// completed. It grows during the run and is derived ONLY from what the game
// has shown. It is never seeded from the ROM's map table or from a list
// written out in advance: seeding it is the difference between an agent that
// explores and an agent that reads our notes.
//
// Adjacency is the one field that is not game-shown, and it is kept apart
// on purpose: it is route geometry (which map's doors lead where), built
// once by Run from the ROM, because "reachable from where you stand" is a
// question about exits and deterministic code keeps the geometry. It names
// no places; it only answers where the doors of a map lead.
type Knowledge struct {
	Visited map[uint8]bool  // maps the player has stood on
	Places  map[string]bool // place names the game has shown (visited or spoken)
	// Completed counts how many times each objective has SUCCEEDED, by
	// Objective.String(). A count, not a flag: "go to pallet town" having
	// worked once and having worked six times are different facts, and the
	// second one is a run walking in circles. Zero is the set test.
	Completed map[string]int
	// Failures are the objectives the run has TRIED and failed, normally by
	// Objective.String(). A leader loss is one scoped exception: KindGym
	// renders "here" in the menu, so an actual loss is keyed by its factual
	// gym place (gymLossFailureKey) to avoid treating Brock and Misty as the
	// same failure. A trainer-caused blackout is another: fight/flee travel
	// variants that hit the same mandatory trainer share one recovery key.
	// It exists because History scrolls: it carries historyCap rounds, so six
	// rounds of anything push a failure out of the planner's view entirely.
	// MEASURED 2026-08-31: "go to route 2" failed on rounds 10 and 11, six
	// talk rounds followed, and by round 19 the model could no longer see it
	// had ever tried — so it tried again, twice, and the run ended on the
	// identical-failure guard. A tally that does not scroll is the difference
	// between "untried idea" and "I have hit this eight times".
	//
	// An entry is dropped the moment the same objective succeeds: a wall
	// that opened is not a wall, and a stale failure would argue against
	// the thing that now works. Scoped combat losses additionally clear after
	// a successful Train rung, because the party has materially changed.
	Failures  map[string]Failure
	Talked    map[uint8]map[[2]uint8]bool // map-local object coordinates already talked to
	Adjacency map[uint8][]uint8           // for each map id, the map ids its exits lead to
	// Requirements are the walls the game has stated: each one the raw
	// sentence, kept VERBATIM — the model reads English, and a parser that
	// turned the sentence into a badge name or item id would be us deciding
	// what it meant — plus WHERE it was heard and HOW MANY times. Like
	// Places, one enters only when the game actually said it. Newest first,
	// deduplicated by text, capped at requirementCap.
	Requirements []Requirement
}

// NewKnowledge returns an empty Knowledge over the given route geometry.
// All maps start empty: a fresh run has seen nothing.
func NewKnowledge(adjacency map[uint8][]uint8) *Knowledge {
	return &Knowledge{
		Visited:      map[uint8]bool{},
		Places:       map[string]bool{},
		Completed:    map[string]int{},
		Talked:       map[uint8]map[[2]uint8]bool{},
		Adjacency:    adjacency,
		Requirements: []Requirement{},
		Failures:     map[string]Failure{},
	}
}

// Failure is one objective the run has tried and failed, and how often. The
// error text is the last one verbatim, like Requirement.Text: what went
// wrong is the game's answer, not ours to summarise.
type Failure struct {
	Objective string
	Times     int
	Last      string // the last error, verbatim
}

// Completion is one objective the run has finished, and how many times.
// The count is what separates "this worked" from "this has worked six times
// and having worked six times are different facts, and the
// second one is a run walking in circles. Zero is the set test.
type Completion struct {
	Objective string
	Times     int
}

// failureCap bounds how many distinct failed objectives the observation
// carries, most-failed first. Same shape as requirementCap: enough to show
// what the run keeps walking into, not a transcript of everything that ever
// went wrong.
const failureCap = 8

// Failed records that an objective was tried and failed. The same objective
// failing again is not a new entry — the count goes up and the newest error
// replaces the old one. A reached-and-lost gym battle gets a gym-scoped key;
// a trainer-caused blackout gets a normalized objective-scoped key; other
// errors keep the ordinary objective identity.
func (k *Knowledge) Failed(o Objective, err error) {
	if err == nil {
		return
	}
	name := o.String()
	if gymName, ok := gymLossFailureName(o, err); ok {
		name = gymName
	}
	if trainerName, ok := trainerLossFailureName(o, err); ok {
		name = trainerName
	}
	f := k.Failures[name]
	f.Objective, f.Times, f.Last = name, f.Times+1, err.Error()
	k.Failures[name] = f
}

// FailureList renders the tally for an observation: most-failed first, ties
// by name so the list does not reshuffle between rounds, capped at
// failureCap.
func (k *Knowledge) FailureList() []Failure {
	out := make([]Failure, 0, len(k.Failures))
	for _, f := range k.Failures {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Times != out[j].Times {
			return out[i].Times > out[j].Times
		}
		return out[i].Objective < out[j].Objective
	})
	if len(out) > failureCap {
		out = out[:failureCap]
	}
	return out
}

// Requirement is one wall the game has stated, and the facts about hitting
// it that the game's own words do not carry.
//
// Text alone is not actionable. MEASURED 2026-08-31: the old man blocking
// Viridian's north path says "You can't go through here! This is private
// property!" and nothing else — the decomp moves him on EVENT_GOT_POKEDEX
// (ViridianCityCheckGotPokedexScript), but he never says so. A planner
// handed that sentence has a wall and no key, and it walked into it on ten
// consecutive rounds because nothing in the observation said it had been
// there before.
//
// Place/X/Y and Times are that missing fact: not what the wall MEANS —
// that stays the planner's to work out — but that this exact wall, at this
// exact tile, has now been hit N times. A loop the run cannot see is a loop
// it repeats.
type Requirement struct {
	Text  string // the game's words, verbatim; never parsed
	Place string // map name where it was last heard; "" when unknown
	X, Y  uint8  // tile where it was last heard
	Times int    // how many rounds have heard it, this one included
}

// SawMap records that the player has stood on the map. A map the player has
// stood on is a place the player knows, whatever its name.
func (k *Knowledge) SawMap(id uint8) { k.Visited[id] = true }

// SawDialogue scans decoded dialogue lines for place names and for the raw
// sentences that state a requirement or a blocked way, and keeps whatever it
// finds. The place table is used as a VOCABULARY for the name match — which
// names are recognizable — not as a seed: a name enters Knowledge only when
// the game actually said it. Matching is on word boundaries, so "Route 22"
// does not count as a mention of "route 2".
func (k *Knowledge) SawDialogue(lines []string, place string, x, y uint8) {
	for _, line := range lines {
		low := strings.ToLower(line)
		for _, name := range skill.PlaceNames() {
			if mentions(low, name) {
				k.Places[name] = true
			}
		}
		k.HeardRequirement(line, place, x, y)
	}
}

// requirementCap bounds how many harvested sentences a run carries. A box
// that re-fires every frame while the player stands on its tile must not
// fill the observation with the same sentence forty times: dedup keeps one
// copy, and the cap keeps the prompt bounded even if the game states many
// different walls. Newest first.
const requirementCap = 8

// requirementShapes are ENGLISH SHAPES that mark a line as one where the
// game tells the player what it needs or what blocks it. This list is a
// FILTER, not knowledge: it decides which lines are interesting to carry
// forward, never what is true — it names no badges, items or events, and
// the harvested value is the raw sentence (Knowledge.Requirements), not a
// parsed requirement. Keep it short; every shape here is a whole phrase the
// game actually uses to state a requirement or a wall:
//
//	"You can pass here only if you have the CASCADEBADGE!"  (text/Route23.asm)
//	"You don't have the CASCADEBADGE yet!"                  (text/Route23.asm)
//	"You need to look everywhere to get different kinds!"    (text/ViridianForestNorthGate.asm)
//	"You can't go through here! This is private property!"   (text/ViridianCity.asm)
var requirementShapes = []string{
	"you don't have",
	"you need",
	"only if you have",
	"can't go through",
}

// looksLikeRequirement reports whether a line carries a requirement shape.
// Gen 1 wraps dialogue at the line width, so a shape may be split across a
// newline ("You can't go\nthrough here!"); the match runs on whitespace-
// normalized text. The stored value stays verbatim — normalization is only
// for the test, never for what the planner reads.
func looksLikeRequirement(line string) bool {
	low := strings.ToLower(line)
	for strings.Contains(low, "\n") {
		low = strings.ReplaceAll(low, "\n", " ")
	}
	for strings.Contains(low, "  ") {
		low = strings.ReplaceAll(low, "  ", " ")
	}
	for _, s := range requirementShapes {
		if strings.Contains(low, s) {
			return true
		}
	}
	return false
}

// HeardRequirement records one raw sentence the game has said, if it carries
// a requirement shape, along with where the player stood when it was said.
// The sentence is kept verbatim and deduplicated: a box that re-fires while
// the player stands there must not fill the observation with the same line
// forty times. A line already known is not a new entry — it moves to the
// front, takes the current position, and its Times count goes up, which is
// the whole point: hitting the same wall twice is a fact the run must be
// able to see. The list is capped at requirementCap, newest first.
func (k *Knowledge) HeardRequirement(line, place string, x, y uint8) {
	line = strings.TrimSpace(line)
	if line == "" || !looksLikeRequirement(line) {
		return
	}
	seen := Requirement{Text: line, Place: place, X: x, Y: y, Times: 1}
	for _, prev := range k.Requirements {
		if prev.Text == line {
			seen.Times = prev.Times + 1
			break
		}
	}
	out := make([]Requirement, 0, len(k.Requirements)+1)
	out = append(out, seen)
	for _, prev := range k.Requirements {
		if prev.Text != line && len(out) < requirementCap {
			out = append(out, prev)
		}
	}
	k.Requirements = out
}

// Done records a completed objective. Only one-shot objectives are dropped
// from the menu because of this (see Offer): a completed heal is no reason
// to stop offering heals. A successful Train is also the recovery boundary
// for observed combat losses: it clears scoped leader/trainer-loss failures
// so the materially changed party can try again.
func (k *Knowledge) Done(o Objective) {
	name := o.String()
	k.Completed[name]++
	// It worked: whatever blocked it before is gone, and a kept failure
	// would argue against the thing that just succeeded.
	delete(k.Failures, name)
	delete(k.Failures, trainerLossFailureKey(o))
	if o.Kind == KindGym && o.Place != "" {
		delete(k.Failures, gymLossFailureKey(o.Place))
	}
	if o.Kind == KindTrain {
		k.clearGymLossFailures()
		k.clearTrainerLossFailures()
	}
}

// TalkedTo records a successful conversation at a map-local object tile.
// Coordinates alone are not globally unique: the same (x,y) can name
// unrelated people on different maps.
func (k *Knowledge) TalkedTo(mapID, x, y uint8) {
	if k.Talked[mapID] == nil {
		k.Talked[mapID] = map[[2]uint8]bool{}
	}
	k.Talked[mapID][[2]uint8{x, y}] = true
}

// restore folds one captured memoryFile (memory.go) back into k. It is the
// read-side half of checkpoint resume, and it touches exactly the fields
// the game has shown: maps stood on, place names spoken or visited,
// objectives completed, objects talked to. Adjacency is left as k was built
// with: route geometry is rebuilt from the ROM by world.BuildGraph every
// run, never restored from disk — a stale copy would outlive a map fix.
func (k *Knowledge) restore(mem memoryFile) {
	for _, id := range mem.Visited {
		k.Visited[id] = true
	}
	for _, name := range mem.Places {
		k.Places[name] = true
	}
	for _, c := range mem.Completed {
		k.Completed[c.Objective] = c.Times
	}
	for _, f := range mem.Failures {
		k.Failures[f.Objective] = f
	}
	for _, t := range mem.Talked {
		k.TalkedTo(t.Map, t.X, t.Y)
	}
	for _, r := range mem.Requirements {
		// Through HeardRequirement, not a raw append: the file was written
		// by a version that already filtered these, and re-validating keeps
		// this field honest if the shape list ever shrinks. The restored
		// Times is then put back — a resumed run must not forget it has
		// already hit this wall five times.
		k.HeardRequirement(r.Text, r.Place, r.X, r.Y)
		if len(k.Requirements) > 0 && k.Requirements[0].Text == r.Text {
			k.Requirements[0].Times = r.Times
		}
	}
}

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
// ORDER: the things that act on where the player already is come first —
// the starter, the story errand, a catch, a heal, the gym, training, a
// purchase, the people and items of this map — and the journeys come last.
// Order is not a hint about which is right: every objective is offered
// either way, and the planner still picks freely. It is about what an
// arbitrary order costs. Journeys are the one part of the menu that grows
// with the size of the KNOWN WORLD, and they were listed first, so every
// map the run discovered pushed the verbs further down.
//
// MEASURED 2026-08-31: standing in Oak's lab with the parcel undelivered,
// "deliver oak's parcel" sat at index 9 of 9 — and at index 17 of 17 once
// the errand's own walk through Viridian was recorded, behind sixteen
// near-identical travel lines. A model reading a numbered list top-down had
// the one action that advances the story sink out of reach for no reason
// except that it had seen more of the map. The verbs do not multiply; the
// places do, so the places go at the bottom.
//
// It says what is POSSIBLE, never what is WISE before the run has evidence.
// An underlevelled party is allowed its first gym/trainer attempt: pre-judging
// it would be us playing the game. Once a mandatory trainer actually wins,
// repeating the same logical objective is observed evidence, not an unknown
// choice. That retry stays off the menu until a Train objective succeeds;
// travel fight/flee variants share one gate because trainers cannot be fled.
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
	adjacentMaps := map[uint8]bool{}
	for _, n := range known.Adjacency[obs.Map] {
		knownMaps[n] = true
		adjacentMaps[n] = true
	}
	for name := range known.Places {
		if d, ok := skill.Place(name); ok {
			knownMaps[d.Map] = true
		}
	}
	// Journeys are collected here and appended LAST (see the ordering note
	// in Offer's doc): they are the only part of the menu that grows with
	// the size of the known world, so listing them first buries every verb
	// deeper with every map the run discovers. A directly adjacent map the
	// run has never stood on gets that fact on its own choice line. This is
	// geometry plus run history, not walkthrough knowledge: it says "new
	// exit", never whether taking it is strategically correct.
	journeys := make([]Objective, 0, 8)
	for _, name := range skill.PlaceNames() { // sorted: a stable menu order
		d, _ := skill.Place(name)
		if !knownMaps[d.Map] {
			continue
		}
		if d.Map == obs.Map && d.X == obs.X && d.Y == obs.Y {
			continue // already standing on it; "go" would be a no-op
		}
		// Both variants, side by side: the plain leg fights wild battles,
		// the fleeing one runs them. The choice is made HERE, as an index,
		// not in the reply — the model cannot reliably attach a conditional
		// argument ("flee": true) to only the objectives that carry it, and
		// a misplaced flag used to stop whole runs (see llm.go's schema).
		plain := Objective{Kind: KindGoTo, Place: name}
		flee := Objective{Kind: KindGoTo, Place: name, Flee: true}
		if adjacentMaps[d.Map] && !known.Visited[d.Map] {
			plain.Note = "(unvisited adjacent map)"
			flee.Note = "(unvisited adjacent map)"
		}
		journeys = append(journeys, plain, flee)
	}

	// Verbs, gated on preconditions that are facts about the situation —
	// never judgements about it.
	if observedEvent(obs, state.EventBattledRivalInOaksLab.String()) &&
		known.Completed[Objective{Kind: KindErrand}.String()] == 0 {
		out = append(out, Objective{Kind: KindErrand}) // one-shot story event
	}
	// The Hideout is a second one-shot story verb. It appears only on maps
	// from which the skill can actually begin or resume, including Celadon's
	// Center after a blackout. The positive postcondition is equally factual:
	// once the bag contains the Scope the objective disappears because the
	// ROM has no second Scope to obtain.
	if skill.RocketHideoutAvailable(obs.Map) && !bagHas(obs, "silph scope") {
		out = append(out, Objective{Kind: KindRocketHideout})
	}
	// The Tower follows the Hideout's concrete postcondition: it appears only
	// where the story skill can begin/resume, only once the Scope is actually
	// owned, and disappears once the Poké Flute is actually in the bag.
	if skill.PokemonTowerAvailable(obs.Map) && bagHas(obs, "silph scope") && !bagHas(obs, "poke flute") {
		out = append(out, Objective{Kind: KindPokemonTower})
	}
	// The Fuchsia slice follows the Tower's concrete postcondition. It is
	// offered on every supported resume map once the Poké Flute is owned and
	// remains until Soul Badge + HM03 + HM04 are all visible in observation.
	if skill.FuchsiaProgressionAvailable(obs.Map) && bagHas(obs, "poke flute") &&
		(!hasBadge(obs, state.BadgeSoul) || !bagHas(obs, "hm03") || !bagHas(obs, "hm04")) {
		out = append(out, Objective{Kind: KindFuchsiaProgression})
	}
	// One catch per species this map's grass can actually roll, from the
	// map's own wild data (Observation.WildGrass). No species is special:
	// the menu used to name CATERPIE everywhere, which made the one hunt
	// the Brock slice needed look like the only hunt there is, and offered
	// it on maps whose wild table has never held one. A map with no grass
	// encounters names nothing, so the precondition is a fact rather than
	// a guess, and balls alone are still not enough to hunt.
	if hasBalls(obs) {
		for _, w := range obs.WildGrass {
			if sp, ok := SpeciesByName(w.Name); ok {
				out = append(out, Objective{Kind: KindCatch, Species: sp})
			}
		}
	}
	ppExhausted := leadOutOfPP(obs)
	if isCenter(obs.MapName) {
		heal := Objective{Kind: KindHeal}
		if ppExhausted {
			heal.Note = "(lead has no PP; Center restores PP without spending finite items)"
		}
		out = append(out, heal)
	} else if name, ok := nearestKnownCenter(obs, known, knownMaps); ok && (partyHurt(obs) || ppExhausted) {
		// A hurt or PP-exhausted party in the field has no recovery-shaped
		// option otherwise. Center recovery is the free, renewable resource;
		// when PP is the reason, the note makes the tradeoff against finite
		// Ether/Elixer objectives visible on the exact choice line.
		note := ""
		if ppExhausted {
			note = "(lead has no PP; Center restores PP without spending finite items)"
		}
		out = append(out,
			Objective{Kind: KindHeal, Place: name, Note: note},
			Objective{Kind: KindHeal, Place: name, Flee: true, Note: note},
		)
	}
	// Field medicine: use a bag item on a party member without walking to
	// a Center. On the menu only while BOTH facts hold — the bag holds an
	// item the ROM dispatches to ItemUseMedicine (fieldMedStatus), AND a
	// slot exists it would do something to. A POTION offered to a whole
	// party is a round that changes nothing: the same reason a satisfied
	// starter is withheld and a full party is not offered the travelling
	// heal. The planner picks the item and the target; Offer only says the
	// pair is legal.
	for _, it := range obs.Bag {
		want, ok := fieldMedStatus[it.Name]
		if !ok || it.Quantity < 1 {
			continue // not medicine (or an unknown id "item N"); no use-item for it
		}
		id, _ := ItemByName(it.Name) // every fieldMedStatus key is a table name
		for slot, mon := range obs.Party {
			if medReaches(mon, want) {
				out = append(out, Objective{Kind: KindUseItem, Item: id, Slot: slot})
			}
		}
	}
	// PP recovery items are finite pickups in Red. Offer them only at hard
	// exhaustion and never while already standing in a Center, where the same
	// recovery is free. A known Center may still be some distance away, so in
	// the field both choices remain visible and the finite-resource note tells
	// the planner exactly what it is spending.
	if ppExhausted && len(obs.Party) > 0 && !isCenter(obs.MapName) {
		for _, it := range obs.Bag {
			id, ok := ppRestoreItems[it.Name]
			if !ok || it.Quantity < 1 {
				continue
			}
			out = append(out, Objective{
				Kind: KindUseItem,
				Item: id,
				Slot: 0,
				Note: "(finite PP recovery; prefer a known Center when the detour is practical)",
			})
		}
	}
	// The gym objective fights the leader of whichever gym the player is
	// standing in; elsewhere it has no one to fight. Being on a gym map is
	// a precondition of the verb, not a verdict on whether the party is
	// ready for its FIRST attempt — underlevelled stays offered. An actual
	// leader loss is different evidence: that gym is withheld until a Train
	// objective succeeds, while another gym remains independent. Place is
	// carried as deterministic internal identity; String still says "here".
	// A badge already earned is different again: that leader will not
	// rebattle, so the challenge could only fail.
	if g, ok := skill.GymAt(obs.Map); ok && !hasBadge(obs, g.Badge) {
		gym := Objective{Kind: KindGym, Place: g.Place}
		if _, lost := known.Failures[gymLossFailureKey(g.Place)]; !lost {
			out = append(out, gym)
		}
	}
	// Training is offered wherever the grass can actually roll an encounter
	// and there is a lead to level up. The target used to be a fixed 12 —
	// the level the Brock slice happened to need — which said "train" until
	// 12 and then never again, whatever the run was walking into next. It
	// is now the next rung above the lead, so the objective always names a
	// step the run has not taken; picking it again climbs another rung. The
	// lead level and this map's wild level band are copied onto the choice
	// line itself so the planner does not have to join that fact from a
	// separate observation field. This still makes no judgement about
	// whether training is wise.
	//
	// A lead below Train's retreat line is the one case that is withheld,
	// and it is a FACT rather than a judgement: Train refuses to start from
	// below that line (skill.BelowRetreatLine, the same predicate the
	// session itself uses), so the objective could only ever report a
	// retreat. That is not "unwise", it is "already answered" — the same
	// ground the satisfied starter, the full party's heal and medReaches
	// stand on. MEASURED 2026-08-31: rounds 13 and 14 of the best run to
	// date were back-to-back train retreats from a hurt lead. The line is
	// skill's to own: BelowRetreatLine reads retreatLineNum/Den, the
	// fraction Train enforces, and a lead at or above it is still offered.
	//
	// The planner's correct response is still its own to find: KindHeal and
	// the field medicine are on the menu whenever they would do something,
	// and the party's HP is in the observation either way.
	if obs.HasGrass && len(obs.Party) > 0 {
		lead := obs.Party[0]
		if !skill.BelowRetreatLine(lead.HP, lead.MaxHP) {
			if target := int(lead.Level) + trainStep; target <= 100 {
				out = append(out, Objective{
					Kind:  KindTrain,
					Level: uint8(target),
					Note:  trainingChoiceNote(lead, obs.WildGrass),
				})
			}
		}
	}
	// The shelf is the clerk's own ROM data (Observation.MartStock, decoded
	// by Observe from the clerk's text script — red/rom.MartItems), not a
	// fixed list: the menu used to offer POTION at every mart, and the
	// Viridian Mart does not stock it, so the first shop every run reached
	// carried a guaranteed-failing objective (MEASURED 2026-08-31: the
	// planner picked it, Buy returned ErrNotInStock, and the run died four
	// rounds later). The shelf is then filtered through EconomyContext: only
	// purchases it marks ShouldBuy are offered, at the affordable SuggestedQty
	// that already preserves strategic reserves and bag capacity. A shelf that
	// cannot be read offers NOTHING — an objective that cannot succeed is
	// worse than an absent one: it costs a round, a model call, and (before
	// the shop closed itself) the run.
	if isMart(obs.MapName) {
		if economy := EconomyContext(obs); economy != nil {
			for _, advice := range economy.Purchases {
				if !advice.ShouldBuy || advice.SuggestedQty < 1 {
					continue
				}
				if it, ok := ItemByName(advice.Item); ok {
					out = append(out, Objective{Kind: KindBuy, Item: it, Qty: advice.SuggestedQty})
				}
			}
		}
	}

	// The people and items of this map, from the ROM map header (see
	// MapObjects): each person is a KindTalk at its tile, each item a
	// KindPickup with the item named. Trainers are REPORTED in MapObjects
	// but NOT offered — a fightable trainer verb is a separate slice, and
	// offering one with no verb behind it manufactures a guaranteed failed
	// objective every round. Observe has already removed map objects whose
	// authoritative toggleable-object flag says they are hidden.
	for _, o := range obs.MapObjects {
		switch o.Kind {
		case "person":
			if !known.Talked[obs.Map][[2]uint8{o.X, o.Y}] {
				out = append(out, Objective{Kind: KindTalk, X: o.X, Y: o.Y})
			}
		case "item":
			if id, ok := ItemByName(o.Item); ok {
				out = append(out, Objective{Kind: KindPickup, X: o.X, Y: o.Y, Item: id})
			}
		}
	}
	candidates := append(out, journeys...)
	return annotate(filterTrainerLossBlocked(candidates, known), known)
}

// annotate composes each objective's run history with any factual choice-local
// note Offer already attached. Objective.Note is planner-facing only and
// String() ignores it, so adding facts here never changes objective identity.
//
// The counts were already in the observation (Failures) and in Knowledge
// (Completed), and a model that reads a numbered list top-down skipped
// straight past them: it re-picked "go to route 2" with a Failures entry
// naming it three lines above, and walked Pallet-Lab-Pallet for eight
// rounds while History recorded every leg as "done". A fact the planner has
// to join across two parts of the prompt is a fact it does not use. On the
// line it is choosing, it might. The same principle now carries objective-
// local facts such as "unvisited adjacent map" and the local training band.
//
// This withholds nothing. Every objective passed to annotate is still kept,
// in the same order. Facts make choices distinguishable; they never say which
// one is strategically correct.
func annotate(out []Objective, known *Knowledge) []Objective {
	for i := range out {
		name := out[i].String()
		done, failed := known.Completed[name], known.Failures[name].Times
		history := ""
		switch {
		case done > 0 && failed > 0:
			history = fmt.Sprintf("(done %dx, failed %dx)", done, failed)
		case done > 0:
			history = fmt.Sprintf("(done %dx)", done)
		case failed > 0:
			history = fmt.Sprintf("(failed %dx)", failed)
		}
		if history == "" {
			continue
		}
		if out[i].Note == "" {
			out[i].Note = history
		} else {
			out[i].Note += " " + history
		}
	}
	return out
}

// trainingChoiceNote states the two facts that determine how expensive a
// grass grind is: the lead's current level and the local encounter level
// band. It deliberately does not compare them or label the grind good/bad;
// that judgement remains the planner's. WildGrass is ROM-derived observation
// data, the same source DecisionContext uses.
func trainingChoiceNote(lead PartyMon, wild []WildSpecies) string {
	if len(wild) == 0 {
		return fmt.Sprintf("(lead L%d)", lead.Level)
	}
	minLevel, maxLevel := int(wild[0].MinLevel), int(wild[0].MaxLevel)
	for _, w := range wild[1:] {
		if int(w.MinLevel) < minLevel {
			minLevel = int(w.MinLevel)
		}
		if int(w.MaxLevel) > maxLevel {
			maxLevel = int(w.MaxLevel)
		}
	}
	return fmt.Sprintf("(lead L%d; local wilds L%d-L%d)", lead.Level, minLevel, maxLevel)
}

// hasBadge reports whether the observation already lists a badge. Badges
// reach the observation as names (state.Badge.String()), which is what a
// planner reads, so the comparison is on the name rather than the bit.
func hasBadge(obs Observation, b state.Badge) bool {
	for _, name := range obs.Badges {
		if name == b.String() {
			return true
		}
	}
	return false
}

// trainStep is how far above the lead the offered training target sits. Two
// levels is a step the run can finish in a handful of battles and then be
// offered again; a distant target is one long objective whose failure says
// nothing about which part of it went wrong.
//
// SETTLED IN S10-1: with the reply argument-free, one rung per round is the
// right granularity, and this value stays 2. A long climb costs more rounds
// (each is a model call), but a bigger rung just moves the cost into a
// longer, less attributable objective, and the offered target is always a
// step the run has not taken — climbing is choosing Train again, which the
// model answers reliably because it is the same index it already picked. If
// a climb ever proves too slow, the answer is ANOTHER MENU ENTRY (a second
// Train objective at a larger step), never another reply field: adding an
// entry reuses the index, and the per-Kind schema test in llm_test.go would
// fail a reply field.
const trainStep = 2

// monHurt says one mon is fainted or down to half HP or less. Half, not
// "any damage at all": a lead at 25/26 is not a reason to spend a round
// walking, and the point of the offer is the party that will lose the next
// fight it takes.
func monHurt(mon PartyMon) bool {
	return mon.MaxHP > 0 && mon.HP*2 <= mon.MaxHP
}

// partyHurt says at least one mon is fainted or down to half HP or less.
func partyHurt(obs Observation) bool {
	for _, mon := range obs.Party {
		if monHurt(mon) {
			return true
		}
	}
	return false
}

// leadOutOfPP is the hard-exhaustion fact used by recovery offering. Empty
// LeadMoves means there is no reliable move state to act on; otherwise every
// known lead move must be at exactly zero current PP.
func leadOutOfPP(obs Observation) bool {
	if len(obs.LeadMoves) == 0 {
		return false
	}
	for _, move := range obs.LeadMoves {
		if move.PP > 0 {
			return false
		}
	}
	return true
}

// fieldMedStatus says what each field medicine does to a party member, and
// so which mon it would do something TO. Derived from the ROM's own
// dispatch, not typed: ItemUsePtrTable (pokered/engine/items/item_effects.asm)
// sends exactly these items to ItemUseMedicine, and ItemUseMedicine's
// .checkItemType is the per-item rule — POTION through FULL_RESTORE take the
// .healHP path (a fainted mon is refused with .healingItemNoEffect), while
// ANTIDOTE/BURN_HEAL/ICE_HEAL/AWAKENING/PARLYZ_HEAL take
// .cureStatusAilment, each effective only when the mon carries the status
// its mask names (PSN/BRN/FRZ/SLP_MASK/PAR). The values are
// state.Mon.StatusName's vocabulary; "" is an HP healer.
var fieldMedStatus = map[string]string{
	"potion":       "",
	"super potion": "",
	"hyper potion": "",
	"max potion":   "",
	"full restore": "",
	"antidote":     "poisoned",
	"burn heal":    "burned",
	"ice heal":     "frozen",
	"awakening":    "asleep",
	"parlyz heal":  "paralyzed",
}

// medReaches says whether a field medicine does something to this mon: an
// HP healer wants a hurt-but-alive mon (monHurt's rule per slot, minus
// fainted — ItemUseMedicine refuses a potion on a fainted mon, so offering
// one there would change nothing); a status cure wants the mon to carry
// exactly the status it cures.
func medReaches(mon PartyMon, wantStatus string) bool {
	if wantStatus == "" {
		return mon.HP > 0 && monHurt(mon)
	}
	return mon.Status == wantStatus
}

// nearestKnownCenter names the Pokemon Center the player could plausibly
// walk back to: fewest map hops from where they stand, among the centers
// whose maps are already in knownMaps. Knowledge is unchanged by this — a
// center the run has never been inside is not offered, exactly as its GoTo
// is not. Only the hop count comes from the route geometry, which is what
// Adjacency is for (see the Knowledge doc): it decides WHICH known center
// is nearest, never whether an unknown one exists.
func nearestKnownCenter(obs Observation, known *Knowledge, knownMaps map[uint8]bool) (string, bool) {
	dist := map[uint8]int{obs.Map: 0}
	for queue := []uint8{obs.Map}; len(queue) > 0; queue = queue[1:] {
		for _, next := range known.Adjacency[queue[0]] {
			if _, seen := dist[next]; !seen {
				dist[next] = dist[queue[0]] + 1
				queue = append(queue, next)
			}
		}
	}
	best, bestDist := "", 0
	for _, name := range skill.PlaceNames() { // sorted: ties break the same way every round
		d, _ := skill.Place(name)
		if d.Map == obs.Map || !knownMaps[d.Map] || !isCenter(state.MapName(d.Map)) {
			continue
		}
		hops, reachable := dist[d.Map]
		if !reachable {
			continue
		}
		if best == "" || hops < bestDist {
			best, bestDist = name, hops
		}
	}
	return best, best != ""
}

func observedEvent(obs Observation, name string) bool {
	for _, event := range obs.Events {
		if event == name {
			return true
		}
	}
	return false
}

// bagHas reports whether the named item is currently in the observation's
// decoded bag. Names are the same item vocabulary ItemName exposes.
func bagHas(obs Observation, name string) bool {
	for _, it := range obs.Bag {
		if it.Name == name && it.Quantity > 0 {
			return true
		}
	}
	return false
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
