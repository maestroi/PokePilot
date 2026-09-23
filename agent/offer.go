package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// Knowledge is run-owned evidence. Durable geographic knowledge is keyed by
// semantic LocationID rather than a game's native map representation. Native
// map translation is retained only as a runtime compatibility table for old
// checkpoints and transient emulator samples; it is never serialized.
type Knowledge struct {
	Visited      map[LocationID]bool
	Places       map[string]bool
	Completed    map[string]int
	Failures     map[string]Failure
	Talked       map[LocationID]map[[2]uint8]bool
	Adjacency    map[LocationID][]LocationID
	Requirements []Requirement

	// Build is this process's running binary identity (e.g. a git SHA), set
	// by the caller. Empty means unknown, which leaves failure tallies
	// build-unaware (their historical pre-feature behavior).
	Build string

	nativeLocations map[uint8]LocationID
}

// NewKnowledge accepts semantic topology in production and the historical
// map[uint8][]uint8 shape as a one-release compatibility shim for tests/tools.
// Regardless of input, the Knowledge object itself contains semantic keys.
func NewKnowledge(topology any) *Knowledge {
	resolved := KnowledgeTopology{Adjacency: map[LocationID][]LocationID{}, NativeLocations: map[uint8]LocationID{}}
	switch value := topology.(type) {
	case nil:
	case KnowledgeTopology:
		resolved = normalizeKnowledgeTopology(value)
	case map[LocationID][]LocationID:
		resolved = normalizeKnowledgeTopology(KnowledgeTopology{Adjacency: value})
	case map[uint8][]uint8:
		resolved = legacyKnowledgeTopology(value)
	default:
		panic(fmt.Sprintf("agent: unsupported knowledge topology %T", topology))
	}
	return &Knowledge{
		Visited:         map[LocationID]bool{},
		Places:          map[string]bool{},
		Completed:       map[string]int{},
		Talked:          map[LocationID]map[[2]uint8]bool{},
		Adjacency:       resolved.Adjacency,
		Requirements:    []Requirement{},
		Failures:        map[string]Failure{},
		nativeLocations: resolved.NativeLocations,
	}
}

func (k *Knowledge) locationForNative(id uint8) LocationID {
	if k != nil {
		if location := k.nativeLocations[id]; location != "" {
			return location
		}
	}
	return legacyLocationID(id)
}

func (k *Knowledge) nativeAdjacency() map[uint8][]uint8 {
	out := map[uint8][]uint8{}
	if k == nil || len(k.nativeLocations) == 0 {
		return out
	}
	inverse := make(map[LocationID]uint8, len(k.nativeLocations))
	for native, semantic := range k.nativeLocations {
		inverse[semantic] = native
	}
	for from, neighbors := range k.Adjacency {
		fromNative, ok := inverse[from]
		if !ok {
			continue
		}
		for _, to := range neighbors {
			if toNative, ok := inverse[to]; ok {
				out[fromNative] = append(out[fromNative], toNative)
			}
		}
	}
	return out
}

type Failure struct {
	Objective string
	Times     int
	Last      string
	Build     string
}

type Completion struct {
	Objective string
	Times     int
}

const failureCap = 8

func (k *Knowledge) completionCount(o Objective) int {
	if k == nil {
		return 0
	}
	if n, ok := k.Completed[objectiveStorageKey(o)]; ok {
		return n
	}
	if o.Kind == KindGym {
		return 0
	}
	return k.Completed[o.String()]
}

func (k *Knowledge) ordinaryFailure(o Objective) (Failure, bool) {
	if k == nil {
		return Failure{}, false
	}
	if f, ok := k.Failures[objectiveStorageKey(o)]; ok {
		return f, true
	}
	if o.Kind == KindGym {
		return Failure{}, false
	}
	f, ok := k.Failures[o.String()]
	return f, ok
}

func (k *Knowledge) Failed(o Objective, err error) {
	if err == nil {
		return
	}
	storage := objectiveStorageKey(o)
	if generic, ok := combatLossFailureName(o, err); ok {
		storage = generic
		delete(k.Failures, combatRetryReadyKey(o))
	}
	f := k.Failures[storage]
	k.bumpFailureTimes(&f)
	f.Objective, f.Last = o.String(), conciseObjectiveError(o, err)
	k.Failures[storage] = f
}

// bumpFailureTimes increments f.Times as evidence toward "this objective is
// broken". A tally carried over from a different build is not that evidence
// (architecture: repeats only count "same objective, same relevant state,
// same build"), so a build change resets it to a fresh count of one instead
// of letting stale pre-fix failures keep discouraging a step forever.
func (k *Knowledge) bumpFailureTimes(f *Failure) {
	if build := strings.TrimSpace(k.Build); build != "" && f.Build != build {
		f.Times, f.Build = 0, build
	}
	f.Times++
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

func (k *Knowledge) SawLocation(id LocationID) {
	if k != nil && id != "" {
		k.Visited[id] = true
	}
}

// SawMap is retained for old tests and transient native sampler output. New
// runtime policy records Observation.Location through SawLocation.
func (k *Knowledge) SawMap(id any) {
	switch value := id.(type) {
	case LocationID:
		k.SawLocation(value)
	case string:
		k.SawLocation(LocationID(value))
	case uint8:
		k.SawLocation(k.locationForNative(value))
	case int:
		if value >= 0 && value <= 0xff {
			k.SawLocation(k.locationForNative(uint8(value)))
		}
	}
}

const journeyPlaceLimit = 8

func mapHops[K comparable](adjacency map[K][]K, from K) map[K]int {
	hops := map[K]int{from: 0}
	frontier := []K{from}
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

func placeHops(hops map[LocationID]int, name string, catalog ObjectiveCatalog) int {
	destination, ok := catalog.destination(PlaceID(name))
	if !ok || destination.Location == "" {
		return 1 << 30
	}
	h, ok := hops[destination.Location]
	if !ok {
		return 1 << 30
	}
	return h
}

func hasUnvisitedNeighbor(id LocationID, known *Knowledge) bool {
	for _, n := range known.Adjacency[id] {
		if !known.Visited[n] {
			return true
		}
	}
	return false
}

func selectJourneyPlaces(placeNames []string, known *Knowledge, hops map[LocationID]int, catalog ObjectiveCatalog) []string {
	if known == nil || len(known.Adjacency) == 0 || len(placeNames) <= journeyPlaceLimit {
		return placeNames
	}
	var frontier, unvisited, hinterland []string
	for _, name := range placeNames {
		destination, ok := catalog.destination(PlaceID(name))
		switch {
		case !ok || destination.Location == "":
			hinterland = append(hinterland, name)
		case !known.Visited[destination.Location]:
			unvisited = append(unvisited, name)
		case hasUnvisitedNeighbor(destination.Location, known):
			frontier = append(frontier, name)
		default:
			hinterland = append(hinterland, name)
		}
	}
	byHops := func(names []string) {
		sort.SliceStable(names, func(i, j int) bool {
			left, right := placeHops(hops, names[i], catalog), placeHops(hops, names[j], catalog)
			if left != right {
				return left < right
			}
			return names[i] < names[j]
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
	storage := objectiveStorageKey(o)
	legacy := o.String()
	if old := k.Completed[legacy]; old > 0 && legacy != storage {
		k.Completed[storage] += old
		delete(k.Completed, legacy)
	}
	k.Completed[storage]++
	delete(k.Failures, storage)
	delete(k.Failures, legacy)
	delete(k.Failures, combatLossFailureKey(o))
	delete(k.Failures, combatRetryReadyKey(o))
	delete(k.Failures, trainerLossFailureKey(o))
	delete(k.Failures, legacyTrainerLossFailureKey(o))
	if o.Kind == KindGym && o.Place != "" {
		delete(k.Failures, gymLossFailureKey(string(o.Place)))
		delete(k.Failures, legacyGymLossFailureKey(string(o.Place)))
		delete(k.Failures, gymRetryReadyKey(string(o.Place)))
		delete(k.Failures, (Objective{Kind: KindGym}).String())
	}
	if o.Kind == KindTrain {
		k.releaseCombatLossGates()
	}
}

func (k *Knowledge) TalkedAt(location LocationID, x, y uint8) {
	if location == "" {
		return
	}
	if k.Talked[location] == nil {
		k.Talked[location] = map[[2]uint8]bool{}
	}
	k.Talked[location][[2]uint8{x, y}] = true
}

func (k *Knowledge) TalkedTo(location any, x, y uint8) {
	switch value := location.(type) {
	case LocationID:
		k.TalkedAt(value, x, y)
	case string:
		k.TalkedAt(LocationID(value), x, y)
	case uint8:
		k.TalkedAt(k.locationForNative(value), x, y)
	case int:
		if value >= 0 && value <= 0xff {
			k.TalkedAt(k.locationForNative(uint8(value)), x, y)
		}
	}
}

func (k *Knowledge) restore(mem memoryFile) {
	for _, id := range mem.Visited {
		k.SawLocation(id)
	}
	for _, name := range mem.Places {
		k.Places[name] = true
	}
	for _, c := range mem.Completed {
		if c.Key != (ObjectiveKey{}) {
			k.Completed[c.Key.ID()] = c.Times
		} else if c.Objective != "" {
			k.Completed[c.Objective] = c.Times
		}
	}
	for _, f := range mem.Failures {
		failure := Failure{Objective: f.Objective, Times: f.Times, Last: f.Last, Build: f.Build}
		if f.Key != (ObjectiveKey{}) {
			k.Failures[failureStorageKey(f.Key, f.Mode)] = failure
		} else if f.Objective != "" {
			k.Failures[f.Objective] = failure
		}
	}
	for _, t := range mem.Talked {
		k.TalkedAt(t.Location, t.X, t.Y)
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

// Offer is the candidate-only compatibility facade. Provider composition,
// prerequisite evidence, ordering and shared filters live in OfferWithEvidence;
// adding a new objective family no longer requires editing this function.
func Offer(obs Observation, known *Knowledge) []Objective {
	return OfferWithEvidence(obs, known).Candidates
}

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
		done := known.completionCount(out[i])
		failure, _ := known.ordinaryFailure(out[i])
		failed := failure.Times
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
