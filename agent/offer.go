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
	// Objective.String(). Leader losses and trainer-caused blackouts use
	// scoped keys so logically equivalent retries can share one recovery gate.
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
// replaces the old one. Reached-and-lost gym leaders and trainer-caused
// blackouts get scoped recovery identities; other failures keep String().
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

var requirementShapes = []string{
	"you don't have",
	"you need",
	"only if you have",
	"can't go through",
}

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

// Done records a completed objective. Repeatable verbs remain repeatable.
// Successful training clears both leader-loss and ordinary trainer-loss
// recovery gates because the party has materially changed before retrying.
func (k *Knowledge) Done(o Objective) {
	name := o.String()
	k.Completed[name]++
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

func (k *Knowledge) TalkedTo(mapID, x, y uint8) {
	if k.Talked[mapID] == nil {
		k.Talked[mapID] = map[[2]uint8]bool{}
	}
	k.Talked[mapID][[2]uint8{x, y}] = true
}

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
		k.HeardRequirement(r.Text, r.Place, r.X, r.Y)
		if len(k.Requirements) > 0 && k.Requirements[0].Text == r.Text {
			k.Requirements[0].Times = r.Times
		}
	}
}

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

// Offer builds the objectives that make sense HERE, NOW. It reports legal
// choices without pre-judging a first attempt, but once the run has observed
// a leader or mandatory trainer defeat it withholds that unchanged retry
// until training changes the party.
func Offer(obs Observation, known *Knowledge) []Objective {
	out := make([]Objective, 0, 8)

	if obs.PartyCount == 0 {
		out = append(out,
			Objective{Kind: KindStarter, Starter: skill.StarterCharmander},
			Objective{Kind: KindStarter, Starter: skill.StarterSquirtle},
			Objective{Kind: KindStarter, Starter: skill.StarterBulbasaur},
		)
	}

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

	journeys := make([]Objective, 0, 8)
	for _, name := range skill.PlaceNames() {
		d, _ := skill.Place(name)
		if !knownMaps[d.Map] {
			continue
		}
		if d.Map == obs.Map && d.X == obs.X && d.Y == obs.Y {
			continue
		}
		plain := Objective{Kind: KindGoTo, Place: name}
		flee := Objective{Kind: KindGoTo, Place: name, Flee: true}
		if adjacentMaps[d.Map] && !known.Visited[d.Map] {
			plain.Note = "(unvisited adjacent map)"
			flee.Note = "(unvisited adjacent map)"
		}
		journeys = append(journeys, plain, flee)
	}

	if observedEvent(obs, state.EventBattledRivalInOaksLab.String()) &&
		known.Completed[Objective{Kind: KindErrand}.String()] == 0 {
		out = append(out, Objective{Kind: KindErrand})
	}
	if skill.RocketHideoutAvailable(obs.Map) && !bagHas(obs, "silph scope") {
		out = append(out, Objective{Kind: KindRocketHideout})
	}
	if skill.PokemonTowerAvailable(obs.Map) && bagHas(obs, "silph scope") && !bagHas(obs, "poke flute") {
		out = append(out, Objective{Kind: KindPokemonTower})
	}
	if skill.FuchsiaProgressionAvailable(obs.Map) && bagHas(obs, "poke flute") &&
		(!hasBadge(obs, state.BadgeSoul) || !bagHas(obs, "hm03") || !bagHas(obs, "hm04")) {
		out = append(out, Objective{Kind: KindFuchsiaProgression})
	}
	if hasBalls(obs) {
		for _, w := range obs.WildGrass {
			if sp, ok := SpeciesByName(w.Name); ok {
				out = append(out, Objective{Kind: KindCatch, Species: sp})
			}
		}
	}
	if isCenter(obs.MapName) {
		out = append(out, Objective{Kind: KindHeal})
	} else if name, ok := nearestKnownCenter(obs, known, knownMaps); ok && partyHurt(obs) {
		out = append(out,
			Objective{Kind: KindHeal, Place: name},
			Objective{Kind: KindHeal, Place: name, Flee: true},
		)
	}
	for _, it := range obs.Bag {
		want, ok := fieldMedStatus[it.Name]
		if !ok || it.Quantity < 1 {
			continue
		}
		id, _ := ItemByName(it.Name)
		for slot, mon := range obs.Party {
			if medReaches(mon, want) {
				out = append(out, Objective{Kind: KindUseItem, Item: id, Slot: slot})
			}
		}
	}
	if g, ok := skill.GymAt(obs.Map); ok && !hasBadge(obs, g.Badge) {
		gym := Objective{Kind: KindGym, Place: g.Place}
		if _, lost := known.Failures[gymLossFailureKey(g.Place)]; !lost {
			out = append(out, gym)
		}
	}
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
	if isMart(obs.MapName) {
		for _, name := range obs.MartStock {
			if it, ok := ItemByName(name); ok {
				out = append(out, Objective{Kind: KindBuy, Item: it, Qty: 3})
			}
		}
	}
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

func hasBadge(obs Observation, b state.Badge) bool {
	for _, name := range obs.Badges {
		if name == b.String() {
			return true
		}
	}
	return false
}

const trainStep = 2

func monHurt(mon PartyMon) bool {
	return mon.MaxHP > 0 && mon.HP*2 <= mon.MaxHP
}

func partyHurt(obs Observation) bool {
	for _, mon := range obs.Party {
		if monHurt(mon) {
			return true
		}
	}
	return false
}

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

func medReaches(mon PartyMon, wantStatus string) bool {
	if wantStatus == "" {
		return mon.HP > 0 && monHurt(mon)
	}
	return mon.Status == wantStatus
}

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
	for _, name := range skill.PlaceNames() {
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

func bagHas(obs Observation, name string) bool {
	for _, it := range obs.Bag {
		if it.Name == name && it.Quantity > 0 {
			return true
		}
	}
	return false
}

func hasBalls(obs Observation) bool {
	for _, it := range obs.Bag {
		if it.Name == "pokeball" || it.Name == "great ball" {
			return true
		}
	}
	return false
}

func isCenter(mapName string) bool {
	return strings.Contains(strings.ToUpper(mapName), "POKECENTER")
}

func isMart(mapName string) bool {
	return strings.Contains(strings.ToUpper(mapName), "MART")
}
