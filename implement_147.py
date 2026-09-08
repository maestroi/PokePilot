from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    s = p.read_text()
    if old not in s:
        raise SystemExit(f"missing replacement anchor in {path}: {old[:100]!r}")
    if s.count(old) != 1:
        raise SystemExit(f"replacement anchor not unique in {path}: {old[:100]!r} ({s.count(old)})")
    p.write_text(s.replace(old, new, 1))


# Observation carries structured semantic route blockages to every planner.
replace_once("agent/observe.go", 'import (\n\t"sort"\n\n', 'import (\n')
replace_once("agent/observe.go",
'''\tRequirements []Requirement
\tUnroutable   []string `json:"-"`
}''',
'''\tRequirements   []Requirement
\tRouteBlockages []RouteBlockage
\tUnroutable     []string `json:"-"`
}''')
replace_once("agent/observe.go",
'''type FieldCapability struct {
\tName       CapabilityID
\tBadge      string
\tBadgeOwned bool
\tHMOwned    bool
\tLearned    bool
\tPartySlot  int
\tUsable     bool
}''',
'''type FieldCapability struct {
\tName       CapabilityID
\tBadge      string
\tBadgeOwned bool
\tHMOwned    bool
\tLearned    bool
\tPartySlot  int
\tUsable     bool
\tPreparable bool
}''')
replace_once("agent/observe.go",
'''\tfor _, cap := range skill.FieldCapabilities(&mem) {
\t\tobs.FieldCapabilities = append(obs.FieldCapabilities, FieldCapability{
\t\t\tName:       CapabilityID(semanticPlace(cap.Name)),
\t\t\tBadge:      cap.Badge.String(),
\t\t\tBadgeOwned: cap.BadgeOwned,
\t\t\tHMOwned:    cap.HMOwned,
\t\t\tLearned:    cap.Learned,
\t\t\tPartySlot:  cap.PartySlot,
\t\t\tUsable:     cap.Usable,
\t\t})
\t}''',
'''\tfor _, cap := range skill.FieldCapabilities(&mem) {
\t\tobs.FieldCapabilities = append(obs.FieldCapabilities, FieldCapability{
\t\t\tName:       CapabilityID(semanticPlace(cap.Name)),
\t\t\tBadge:      cap.Badge.String(),
\t\t\tBadgeOwned: cap.BadgeOwned,
\t\t\tHMOwned:    cap.HMOwned,
\t\t\tLearned:    cap.Learned,
\t\t\tPartySlot:  cap.PartySlot,
\t\t\tUsable:     cap.Usable,
\t\t\tPreparable: cap.Usable || skill.CanPrepareFieldMove(romData, &mem, cap.Move),
\t\t})
\t}''')
replace_once("agent/observe.go",
'''\tobs.Unroutable = unroutablePlaces(m, romData)
\tobs.WildGrass = []WildSpecies{}''',
'''\troutes := routeAvailabilityFor(m, romData)
\tobs.Unroutable = routes.Unroutable
\tobs.RouteBlockages = routes.Blockages
\tobs.WildGrass = []WildSpecies{}''')
replace_once("agent/observe.go",
'''func unroutablePlaces(m *emu.Emu, romData []byte) []string {
\tplanner, err := skill.NewRoutePlanner(m, romData)
\tif err != nil {
\t\treturn nil
\t}
\tvar out []string
\tfor _, name := range skill.PlaceNames() {
\t\td, ok := skill.Place(name)
\t\tif !ok || planner.CanReach(d) {
\t\t\tcontinue
\t\t}
\t\tout = append(out, name)
\t}
\tif out == nil {
\t\treturn []string{}
\t}
\tsort.Strings(out)
\treturn out
}''',
'''func unroutablePlaces(m *emu.Emu, romData []byte) []string {
\treturn routeAvailabilityFor(m, romData).Unroutable
}''')

# Semantic blockages are never executable immediate journeys. Geometry-only
# failures retain the old fail-open safety valve for a bad live topology read.
replace_once("agent/offer.go",
'''\tjourneys := make([]Objective, 0, 2*journeyPlaceLimit)
\thops := mapHops(known.Adjacency, obs.Map)
\tunroutable := map[string]bool{}
\tfor _, name := range obs.Unroutable {
\t\tunroutable[name] = true
\t}
\tplaceNames := make([]string, 0, 16)''',
'''\tjourneys := make([]Objective, 0, 2*journeyPlaceLimit)
\thops := mapHops(known.Adjacency, obs.Map)
\tsemanticBlocked := map[string]bool{}
\tfor _, blockage := range obs.RouteBlockages {
\t\tsemanticBlocked[string(blockage.Destination)] = true
\t}
\tunroutable := map[string]bool{}
\tfor _, name := range obs.Unroutable {
\t\tif !semanticBlocked[name] {
\t\t\tunroutable[name] = true
\t\t}
\t}
\tplaceNames := make([]string, 0, 16)''')
replace_once("agent/offer.go",
'''\tif len(unroutable) > 0 {
\t\troutable := make([]string, 0, len(placeNames))''',
'''\tif len(semanticBlocked) > 0 {
\t\troutable := make([]string, 0, len(placeNames))
\t\tfor _, name := range placeNames {
\t\t\tif !semanticBlocked[name] {
\t\t\t\troutable = append(routable, name)
\t\t\t}
\t\t}
\t\tplaceNames = routable
\t}
\tif len(unroutable) > 0 {
\t\troutable := make([]string, 0, len(placeNames))''')

Path("agent/route_blockage.go").write_text('''package agent

import (
    "errors"
    "sort"

    "github.com/maestroi/pokepilot/emu"
    gameruntime "github.com/maestroi/pokepilot/game"
    "github.com/maestroi/pokepilot/skill"
    "github.com/maestroi/pokepilot/world"
)

const routeBlockageCap = 12

// RoutePrerequisiteLink connects a missing portable route capability to
// planner-visible state that already describes how close the current snapshot
// is to satisfying it. It names conditions, never a game-specific recipe.
type RoutePrerequisiteLink struct {
    Capability      CapabilityID `json:"capability"`
    FieldCapability CapabilityID `json:"field_capability,omitempty"`
    Progress        ProgressID   `json:"progress,omitempty"`
}

// RouteBlockage is the bounded planner-facing projection of a structured
// world.RouteBlockedError. Destination is the requested semantic place;
// Transitions and Missing preserve semantic identities without leaking map IDs
// or parsing error prose.
type RouteBlockage struct {
    Destination   PlaceID                 `json:"destination"`
    Transitions   []string                `json:"transitions,omitempty"`
    Missing       []CapabilityID          `json:"missing"`
    Prerequisites []RoutePrerequisiteLink `json:"prerequisites,omitempty"`
}

type routeAvailability struct {
    Unroutable []string
    Blockages  []RouteBlockage
}

type routeReachability interface {
    Reachability(skill.Destination) error
}

func routeAvailabilityFor(m *emu.Emu, romData []byte) routeAvailability {
    planner, err := skill.NewRoutePlanner(m, romData)
    if err != nil {
        // nil means the question was never reliably asked. Offer deliberately
        // fails open on that distinction.
        return routeAvailability{}
    }
    return collectRouteAvailability(planner, skill.PlaceNames())
}

func collectRouteAvailability(planner routeReachability, names []string) routeAvailability {
    if planner == nil {
        return routeAvailability{}
    }
    names = append([]string(nil), names...)
    sort.Strings(names)
    out := routeAvailability{Unroutable: []string{}, Blockages: []RouteBlockage{}}
    for _, name := range names {
        destination, ok := skill.Place(name)
        if !ok {
            continue
        }
        err := planner.Reachability(destination)
        if err == nil {
            continue
        }
        out.Unroutable = append(out.Unroutable, name)

        var blocked *world.RouteBlockedError
        if !errors.As(err, &blocked) || len(out.Blockages) >= routeBlockageCap {
            continue
        }
        blockage := plannerRouteBlockage(semanticPlace(name), blocked)
        if len(blockage.Missing) != 0 {
            out.Blockages = append(out.Blockages, blockage)
        }
    }
    return out
}

func plannerRouteBlockage(destination PlaceID, blocked *world.RouteBlockedError) RouteBlockage {
    result := RouteBlockage{Destination: destination, Transitions: []string{}, Missing: []CapabilityID{}}
    if blocked == nil {
        return result
    }
    transitionSeen := map[string]bool{}
    missingSeen := map[CapabilityID]bool{}
    for _, blockage := range blocked.Blockages {
        if id := blockage.Transition.ID; id != "" && !transitionSeen[id] {
            transitionSeen[id] = true
            result.Transitions = append(result.Transitions, id)
        }
        for _, raw := range blockage.Missing {
            id := CapabilityID(raw)
            if id == "" || missingSeen[id] {
                continue
            }
            missingSeen[id] = true
            result.Missing = append(result.Missing, id)
        }
    }
    sort.Strings(result.Transitions)
    sort.Slice(result.Missing, func(i, j int) bool { return result.Missing[i] < result.Missing[j] })
    for _, id := range result.Missing {
        if link, ok := redRoutePrerequisiteLink(id); ok {
            result.Prerequisites = append(result.Prerequisites, link)
        }
    }
    return result
}

func redRoutePrerequisiteLink(id CapabilityID) (RoutePrerequisiteLink, bool) {
    switch gameruntime.CapabilityID(id) {
    case "can_cut":
        return RoutePrerequisiteLink{Capability: id, FieldCapability: "cut"}, true
    case "can_surf":
        return RoutePrerequisiteLink{Capability: id, FieldCapability: "surf"}, true
    case "can_move_boulders":
        return RoutePrerequisiteLink{Capability: id, FieldCapability: "strength"}, true
    case "can_clear_snorlax":
        return RoutePrerequisiteLink{Capability: id, Progress: redProgressPokeFluteAcquired}, true
    default:
        // Unknown capabilities stay explicit in Missing. Not having a known
        // preparation link is evidence we do not know a recipe yet.
        return RoutePrerequisiteLink{}, false
    }
}
''')

Path("agent/route_blockage_test.go").write_text('''package agent

import (
    "encoding/json"
    "errors"
    "strings"
    "testing"

    gameruntime "github.com/maestroi/pokepilot/game"
    "github.com/maestroi/pokepilot/skill"
    "github.com/maestroi/pokepilot/world"
)

type fakeRouteReachability map[skill.Destination]error

func (f fakeRouteReachability) Reachability(d skill.Destination) error { return f[d] }

func blockedRoute(transition string, missing ...gameruntime.CapabilityID) error {
    return &world.RouteBlockedError{Blockages: []gameruntime.TransitionBlockage{{
        Transition: gameruntime.Transition{ID: transition}, Missing: missing,
    }}}
}

func TestCollectRouteAvailabilityPreservesSemanticBlockages(t *testing.T) {
    cinnabar, ok := skill.Place("cinnabar island")
    if !ok {
        t.Fatal("missing cinnabar island fixture")
    }
    vermilion, ok := skill.Place("vermilion gym")
    if !ok {
        t.Fatal("missing vermilion gym fixture")
    }
    route13, ok := skill.Place("route 13")
    if !ok {
        t.Fatal("missing route 13 fixture")
    }
    mtMoon, ok := skill.Place("mt moon 1f")
    if !ok {
        t.Fatal("missing mt moon 1f fixture")
    }

    planner := fakeRouteReachability{
        cinnabar:  blockedRoute("red:route21_surf", "can_surf"),
        vermilion: blockedRoute("red:vermilion_gym_cut", "can_cut"),
        route13:   blockedRoute("red:route12_snorlax", "can_clear_snorlax"),
        mtMoon:    world.ErrNoRoute,
    }
    got := collectRouteAvailability(planner, []string{"route 13", "mt moon 1f", "cinnabar island", "vermilion gym"})
    if len(got.Unroutable) != 4 {
        t.Fatalf("unroutable = %v, want four unavailable destinations", got.Unroutable)
    }
    if len(got.Blockages) != 3 {
        t.Fatalf("blockages = %+v, want three semantic blockages and geometry kept separate", got.Blockages)
    }

    byPlace := map[PlaceID]RouteBlockage{}
    for _, blockage := range got.Blockages {
        byPlace[blockage.Destination] = blockage
    }
    surf := byPlace["cinnabar island"]
    if len(surf.Missing) != 1 || surf.Missing[0] != "can_surf" || len(surf.Prerequisites) != 1 || surf.Prerequisites[0].FieldCapability != "surf" {
        t.Fatalf("surf blockage = %+v", surf)
    }
    cut := byPlace["vermilion gym"]
    if len(cut.Missing) != 1 || cut.Missing[0] != "can_cut" || cut.Prerequisites[0].FieldCapability != "cut" {
        t.Fatalf("cut blockage = %+v", cut)
    }
    snorlax := byPlace["route 13"]
    if len(snorlax.Missing) != 1 || snorlax.Missing[0] != "can_clear_snorlax" || snorlax.Prerequisites[0].Progress != redProgressPokeFluteAcquired {
        t.Fatalf("snorlax blockage = %+v", snorlax)
    }
    if _, geometryWasMisclassified := byPlace["mt moon 1f"]; geometryWasMisclassified {
        t.Fatal("plain ErrNoRoute was exposed as a missing-capability blockage")
    }
}

func TestRouteBlockageDisappearsWhenCapabilityBecomesReachable(t *testing.T) {
    cinnabar, _ := skill.Place("cinnabar island")
    blocked := collectRouteAvailability(fakeRouteReachability{cinnabar: blockedRoute("red:route21_surf", "can_surf")}, []string{"cinnabar island"})
    if len(blocked.Blockages) != 1 {
        t.Fatalf("before capability: %+v", blocked)
    }
    reachable := collectRouteAvailability(fakeRouteReachability{cinnabar: nil}, []string{"cinnabar island"})
    if len(reachable.Blockages) != 0 || len(reachable.Unroutable) != 0 {
        t.Fatalf("fresh reachable snapshot retained stale blockage: %+v", reachable)
    }
}

func TestPlannerRouteBlockageKeepsUnknownPrerequisiteHonest(t *testing.T) {
    err := blockedRoute("portable:unknown_gate", "can_teleport")
    var blocked *world.RouteBlockedError
    if !errors.As(err, &blocked) {
        t.Fatal("test setup did not produce RouteBlockedError")
    }
    got := plannerRouteBlockage("somewhere", blocked)
    if len(got.Missing) != 1 || got.Missing[0] != "can_teleport" {
        t.Fatalf("missing = %v", got.Missing)
    }
    if len(got.Prerequisites) != 0 {
        t.Fatalf("unknown prerequisite fabricated preparation: %+v", got.Prerequisites)
    }
}

func TestRouteBlockagesArePlannerVisibleAndBounded(t *testing.T) {
    obs := Observation{RouteBlockages: []RouteBlockage{{
        Destination: "cinnabar island",
        Transitions: []string{"red:route21_surf"},
        Missing: []CapabilityID{"can_surf"},
        Prerequisites: []RoutePrerequisiteLink{{Capability: "can_surf", FieldCapability: "surf"}},
    }}}
    encoded, err := json.Marshal(obs)
    if err != nil {
        t.Fatal(err)
    }
    text := string(encoded)
    if !strings.Contains(text, `"RouteBlockages":[{"destination":"cinnabar island"`) || !strings.Contains(text, `"missing":["can_surf"]`) {
        t.Fatalf("planner JSON omitted structured route blockage: %s", text)
    }

    names := make([]string, 0, routeBlockageCap+5)
    planner := fakeRouteReachability{}
    for _, name := range skill.PlaceNames() {
        d, ok := skill.Place(name)
        if !ok {
            continue
        }
        names = append(names, name)
        planner[d] = blockedRoute("portable:gate", "can_surf")
        if len(names) == routeBlockageCap+5 {
            break
        }
    }
    got := collectRouteAvailability(planner, names)
    if len(got.Blockages) > routeBlockageCap {
        t.Fatalf("blockage prompt grew to %d entries, cap is %d", len(got.Blockages), routeBlockageCap)
    }
}

func TestOfferAlwaysWithholdsSemanticBlockedDestination(t *testing.T) {
    known := NewKnowledge(map[uint8][]uint8{0x05: {0x5c}, 0x5c: {0x05}})
    known.SawMap(0x05)
    known.SawMap(0x5c)
    obs := Observation{
        Map: 0x05, MapName: "VERMILION_CITY", PartyCount: 1,
        Unroutable: []string{"vermilion gym"},
        RouteBlockages: []RouteBlockage{{Destination: "vermilion gym", Missing: []CapabilityID{"can_cut"}}},
    }
    if offersPlace(Offer(obs, known), "vermilion gym") {
        t.Fatal("semantic-blocked destination was offered as an immediate GoTo")
    }
}
''')
