package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// Knowledge is run-owned evidence. Game facts enter only after the player has
// observed them; route adjacency is deterministic geometry rebuilt each run.
type Knowledge struct {
	Visited      map[uint8]bool
	Places       map[string]bool
	Completed    map[string]int
	Failures     map[string]Failure
	Talked       map[uint8]map[[2]uint8]bool
	Adjacency    map[uint8][]uint8
	Requirements []Requirement
}

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

type Failure struct {
	Objective string
	Times     int
	Last      string
}

type Completion struct {
	Objective string
	Times     int
}

const failureCap = 8

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
	// conciseObjectiveError is also what History's Outcome text uses: this
	// keeps the planner-facing size bounded and the two representations of
	// "the same failure" from disagreeing, instead of Failures carrying the
	// full raw error (route-stall traces run to hundreds of characters).
	f.Objective, f.Times, f.Last = name, f.Times+1, conciseObjectiveError(o, err)
	k.Failures[name] = f
}

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

type Requirement struct {
	Text  string
	Place string
	X, Y  uint8
	Times int
}

func (k *Knowledge) SawMap(id uint8) { k.Visited[id] = true }

const journeyPlaceLimit = 8

func mapHops(adjacency map[uint8][]uint8, from uint8) map[uint8]int {
	hops := map[uint8]int{from: 0}
	frontier := []uint8{from}
	for len(frontier) > 0 {
		cur := frontier[0]
		frontier = frontier[1:]
		for _, n := range adjacency[cur] {
			if _, seen := hops[n]; seen {
				continue
			}
			hops[n] = hops[cur] + 1
			frontier = append(frontier, n)
		}
	}
	return hops
}

func placeHops(hops map[uint8]int, name string) int {
	d, ok := skill.Place(name)
	if !ok {
		return 1 << 30
	}
	h, ok := hops[d.Map]
	if !ok {
		return 1 << 30
	}
	return h
}

func hasUnvisitedNeighbor(id uint8, known *Knowledge) bool {
	for _, n := range known.Adjacency[id] {
		if !known.Visited[n] {
			return true
		}
	}
	return false
}

// selectJourneyPlaces keeps the travel menu bounded without dropping the
// visited frontier: maps already stood on that still border an unvisited
// neighbor. Unvisited discovery fills next, then nearby hinterland. The old
// unvisited-then-nearest trim hid Route 3 behind Pallet/Viridian once eight
// closer names existed (run-1w32fl3ssna3h2y7f2butfdwbr).
func selectJourneyPlaces(placeNames []string, known *Knowledge, hops map[uint8]int) []string {
	if known == nil || len(known.Adjacency) == 0 || len(placeNames) <= journeyPlaceLimit {
		return placeNames
	}
	var frontier, unvisited, hinterland []string
	for _, name := range placeNames {
		d, ok := skill.Place(name)
		switch {
		case !ok:
			hinterland = append(hinterland, name)
		case !known.Visited[d.Map]:
			unvisited = append(unvisited, name)
		case hasUnvisitedNeighbor(d.Map, known):
			frontier = append(frontier, name)
		default:
			hinterland = append(hinterland, name)
		}
	}
	byHops := func(names []string) {
		sort.SliceStable(names, func(i, j int) bool {
			return placeHops(hops, names[i]) < placeHops(hops, names[j])
		})
	}
	byHops(frontier)
	byHops(unvisited)
	byHops(hinterland)

	out := make([]string, 0, journeyPlaceLimit)
	appendCapped := func(names []string) {
		for _, name := range names {
			if len(out) >= journeyPlaceLimit {
				return
			}
			out = append(out, name)
		}
	}
	appendCapped(frontier)
	appendCapped(unvisited)
	appendCapped(hinterland)
	sort.Strings(out)
	return out
}

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

// Offer builds the portable action menu from settled observation and run
// evidence. Named campaign progression is deliberately absent: production
// composes those game-owned goals through OfferWithProgression.
func Offer(obs Observation, known *Knowledge) []Objective {
	// A trainer/gym-loss gate demands a Train recovery. If this map's grass
	// cannot deliver one, fail-open the same retry-due path a successful
	// rung would have created. Maps without a training estimate (cities,
	// gyms, centers) stay locked so an unchanged commute cannot rechallenge.
	if trainingUnviableHere(obs) {
		known.releaseCombatLossGates()
	}

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
		for _, n := range known.Adjacency[m] {
			knownMaps[n] = true
		}
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

	journeys := make([]Objective, 0, 2*journeyPlaceLimit)
	hops := mapHops(known.Adjacency, obs.Map)
	semanticBlocked := map[string]bool{}
	for _, blockage := range obs.RouteBlockages {
		semanticBlocked[string(blockage.Destination)] = true
	}
	unroutable := map[string]bool{}
	for _, name := range obs.Unroutable {
		if !semanticBlocked[name] {
			unroutable[name] = true
		}
	}
	placeNames := make([]string, 0, 16)
	for _, name := range skill.PlaceNames() {
		d, _ := skill.Place(name)
		if !knownMaps[d.Map] {
			continue
		}
		if journeyProgressionBlocked(obs, d.Map) || placeProgressionBlocked(obs, name) {
			continue
		}
		if d.Map == obs.Map && d.X == obs.X && d.Y == obs.Y {
			continue
		}
		placeNames = append(placeNames, name)
	}
	if len(semanticBlocked) > 0 {
		routable := make([]string, 0, len(placeNames))
		for _, name := range placeNames {
			if !semanticBlocked[name] {
				routable = append(routable, name)
			}
		}
		placeNames = routable
	}
	if len(unroutable) > 0 {
		routable := make([]string, 0, len(placeNames))
		for _, name := range placeNames {
			if !unroutable[name] {
				routable = append(routable, name)
			}
		}
		if len(routable) > 0 {
			placeNames = routable
		}
	}
	if len(known.Adjacency) > 0 && len(placeNames) > journeyPlaceLimit {
		placeNames = selectJourneyPlaces(placeNames, known, hops)
	}
	for _, name := range placeNames {
		d, _ := skill.Place(name)
		plain := Objective{Kind: KindGoTo, Place: name}
		flee := Objective{Kind: KindGoTo, Place: name, Flee: true}
		if adjacentMaps[d.Map] && !known.Visited[d.Map] {
			plain.Note = "(unvisited adjacent map)"
			flee.Note = "(unvisited adjacent map)"
		}
		journeys = append(journeys, plain, flee)
	}

	if hasBalls(obs) && obs.HasGrass {
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
		note := ""
		if ppExhausted {
			note = "(lead has no PP; Center restores PP without spending finite items)"
		}
		out = append(out,
			Objective{Kind: KindHeal, Place: name, Note: note},
			Objective{Kind: KindHeal, Place: name, Flee: true, Note: note},
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

	if g, ok := skill.GymAt(obs.Map); ok && !hasBadge(obs, g.Badge) {
		// Journeys to g.Place are already withheld via semanticBlocked, but
		// KindGym is a local verb. Offering it while the gym interior is
		// gated from here lets the planner copy an illegal challenge into a
		// plan (measured: Vermilion City, missing can_cut). Already standing
		// on the gym map is the one case the exterior gate no longer applies.
		//
		// Viridian is different in kind, not degree: its door needs seven
		// badges, a story gate EnterViridianGym cannot clear itself the way
		// EnterVermilionGym clears its Cut prerequisite. Offering the
		// challenge before then is a guaranteed run-ending failure (measured:
		// run-2uhibnzs9erjg189xy4o2jtxo0 round 5, 30 sibling runs), so it
		// stays behind the same journeyProgressionBlocked fact GoTo already
		// uses for this map.
		gymGated := g.Map == viridianGymMap && journeyProgressionBlocked(obs, g.Map)
		if !gymGated && (obs.Map == g.Map || !semanticBlocked[string(g.Place)]) {
			gym := Objective{Kind: KindGym, Place: g.Place}
			if _, lost := known.Failures[gymLossFailureKey(g.Place)]; !lost {
				out = append(out, gym)
			}
		}
	}

	if obs.HasGrass && len(obs.Party) > 0 {
		lead := obs.Party[0]
		if !skill.BelowRetreatLine(lead.HP, lead.MaxHP) {
			if target := int(lead.Level) + trainStep; target <= 100 {
				out = append(out, Objective{
					Kind:  KindTrain,
					Level: uint8(target),
					Note:  trainingChoiceNote(lead, obs.WildGrass, obs.Training),
				})
			}
		}
	}

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

	for _, object := range obs.MapObjects {
		switch object.Kind {
		case "person":
			if !known.Talked[obs.Map][[2]uint8{object.X, object.Y}] {
				out = append(out, Objective{Kind: KindTalk, X: object.X, Y: object.Y})
			}
		case "trainer":
			challenge := Objective{Kind: KindTrainer, X: object.X, Y: object.Y}
			if object.Challengeable && !object.Defeated && known.Completed[challenge.String()] == 0 {
				out = append(out, challenge)
			}
		case "item":
			if id, ok := ItemByName(object.Item); ok {
				out = append(out, Objective{Kind: KindPickup, X: object.X, Y: object.Y, Item: id})
			}
		}
	}

	if o := lastResortEscapeNote(out, journeys, obs.RespawnPlace); o != "" {
		for i := range out {
			if out[i].Kind == KindTrain {
				out[i] = appendObjectiveNote(out[i], o)
			}
		}
	}

	candidates := append(out, journeys...)
	return annotate(filterTrainerLossBlocked(candidates, known), known)
}

// lastResortEscapeNote reports the note to attach to a KindTrain objective
// when it is the only progress-shaped thing Offer found this round: no
// journey, no gym, no trainer challenge, no talk, no catch, no story
// progression. It never claims this IS the right move, only that it is a
// legal one — fighting without fleeing or retreating, even to a loss,
// forces a respawn instead of repeating the same failed local objective.
func lastResortEscapeNote(out, journeys []Objective, respawn PlaceID) string {
	if len(journeys) != 0 || respawn == "" {
		return ""
	}
	for _, o := range out {
		switch o.Kind {
		case KindGoTo, KindGym, KindTrainer, KindTalk, KindCatch, KindProgress:
			return ""
		}
	}
	return fmt.Sprintf("(no reachable journey or challenge this round; fighting here without fleeing or retreating, even to a loss, forces a respawn at %s)", strings.ToUpper(string(respawn)))
}

func annotate(out []Objective, known *Knowledge) []Objective {
	for i := range out {
		name := out[i].String()
		done, failed := known.Completed[name], known.Failures[name].Times
		// KindGym.String() is place-agnostic ("beat the gym leader here"), so
		// Completed[name] is the count of ALL gyms ever cleared this run, not
		// this one. Offer only ever offers a gym while its own badge is still
		// missing (see !hasBadge above), so a "done Nx" here is always about a
		// different gym and falsely reads as "already beaten here".
		if out[i].Kind == KindGym {
			done = 0
		}
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

func trainingChoiceNote(lead PartyMon, wild []WildSpecies, estimate *TrainingEstimate) string {
	detail := fmt.Sprintf("lead L%d", lead.Level)
	if len(wild) > 0 {
		minLevel, maxLevel := int(wild[0].MinLevel), int(wild[0].MaxLevel)
		for _, w := range wild[1:] {
			if int(w.MinLevel) < minLevel {
				minLevel = int(w.MinLevel)
			}
			if int(w.MaxLevel) > maxLevel {
				maxLevel = int(w.MaxLevel)
			}
		}
		detail += fmt.Sprintf("; local wilds L%d-L%d", minLevel, maxLevel)
	}
	if estimate != nil {
		detail += "; " + estimate.Diagnostic()
	}
	return "(" + detail + ")"
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

func leadOutOfPP(obs Observation) bool {
	if len(obs.LeadPP) == 0 {
		return false
	}
	for _, pp := range obs.LeadPP {
		if pp > 0 {
			return false
		}
	}
	return true
}
