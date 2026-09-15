package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

// Observation is the complete planner-facing view of one settled game state.
// Raw game encodings stay available to game-owned runtime code where necessary,
// but they do not cross the planner JSON contract: Location, party species,
// respawn place and progression are semantic values.
type Observation struct {
	// Map is the current profile's opaque native map id and remains runtime-only
	// during the adapter migration. Location is the portable planner identity.
	Map      uint8 `json:"-"`
	Location PlaceID
	MapName  string
	X, Y     uint8
	Facing   string

	Controllable bool
	InBattle     bool
	PartyCount   int
	Party        []PartyMon
	Badges       []string
	Money        uint32
	RespawnPlace PlaceID
	Events       []string
	Story        ProgressState
	BlackedOut   bool

	LeadMoves         []Move
	LeadPP            []uint8
	Bag               []Item
	FieldCapabilities []FieldCapability
	RecentDialogue    []string
	History           []RoundRecord
	Failures          []Failure

	Round      int
	RoundsLeft int
	Intent     string
	IntentAge  int

	PokedexOwned []SpeciesID
	PokedexSeen  []SpeciesID
	Dex          DexCatalog `json:"-"`

	WildGrass  []WildSpecies
	HasGrass   bool
	Training   *TrainingEstimate `json:"training,omitempty"`
	MartStock  []string
	MapObjects []MapObject

	Requirements   []Requirement
	RouteBlockages []RouteBlockage
	Unroutable     []string `json:"-"`
}

type MapObject struct {
	X, Y          uint8
	Kind          string
	Item          string
	Challengeable bool
	Defeated      bool
}

type Move struct {
	Power uint8
	Type  string
}

type Item struct {
	Name     string
	Quantity int
}

type FieldCapability struct {
	Name       CapabilityID
	Badge      string
	BadgeOwned bool
	HMOwned    bool
	Learned    bool
	PartySlot  int
	Usable     bool
	Preparable bool
}

type RoundRecord struct {
	Objective string
	Outcome   string
}

type PartyMon struct {
	Species    SpeciesID
	Level      uint8
	Experience uint32
	HP         uint16
	MaxHP      uint16
	Status     string
}

type WildSpecies struct {
	Name     string
	MinLevel uint8
	MaxLevel uint8
	Slots    int
}

type semanticObservationAdapter interface {
	GameID() game.GameID
	Observe(*emu.Emu, []byte, game.GameProfile) (Observation, error)
}

var semanticObservationAdapters = map[game.GameID]semanticObservationAdapter{}

func registerSemanticObservationAdapter(adapter semanticObservationAdapter) {
	if adapter == nil || adapter.GameID() == "" {
		panic("agent: invalid semantic observation adapter")
	}
	if _, exists := semanticObservationAdapters[adapter.GameID()]; exists {
		panic(fmt.Sprintf("agent: duplicate semantic observation adapter %q", adapter.GameID()))
	}
	semanticObservationAdapters[adapter.GameID()] = adapter
}

func semanticObservationAdapterFor(profile game.GameProfile) (semanticObservationAdapter, bool) {
	if profile == nil {
		return nil, false
	}
	adapter, ok := semanticObservationAdapters[profile.ID()]
	return adapter, ok
}

func Observe(m *emu.Emu, romData []byte) Observation {
	obs, err := ObserveChecked(m, romData)
	if err != nil {
		panic(err)
	}
	return obs
}

// ObserveChecked is deliberately game-agnostic. It identifies the exact game
// profile, selects that game's registered semantic observation adapter, and
// asks the adapter for the complete planner/runtime observation.
func ObserveChecked(m *emu.Emu, romData []byte) (Observation, error) {
	profile, _, err := profiles.Detect(romData)
	if err != nil {
		return Observation{}, fmt.Errorf("agent: observe: %w", err)
	}
	adapter, ok := semanticObservationAdapterFor(profile)
	if !ok {
		return Observation{}, fmt.Errorf("agent: observe %s@%s: no semantic observation adapter registered", profile.ID(), profile.Revision())
	}
	obs, err := adapter.Observe(m, romData, profile)
	if err != nil {
		return Observation{}, fmt.Errorf("agent: observe %s@%s: %w", profile.ID(), profile.Revision(), err)
	}
	return obs, nil
}
