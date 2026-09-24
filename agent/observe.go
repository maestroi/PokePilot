package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

// RuntimeServices describes optional infrastructure available to this agent
// process. These are capabilities of the runtime, not facts decoded from the
// cartridge, so game adapters do not own or infer them.
type RuntimeServices struct {
	VirtualTrader bool `json:"virtual_trader,omitempty"`
}

// Observation is the complete planner-facing view of one settled game state.
// Raw game encodings stay available to game-owned runtime code where necessary,
// but they do not cross the planner JSON contract: Location, party species,
// respawn place and progression are semantic values.
type Observation struct {
	// GameID selects game-owned catalogs/capabilities without branching on a
	// concrete game in generic policy. Map is the profile's opaque native map
	// id during the final location migration; Location is the portable identity.
	GameID   game.GameID `json:"-"`
	Map      uint8       `json:"-"`
	Location PlaceID
	MapName  string
	X, Y     uint8
	Facing   string

	Controllable       bool
	InBattle           bool
	PartyCount         int
	Party              []PartyMon
	Badges             []string
	Money              uint32
	RespawnPlace       PlaceID
	RecoveryCheckpoint PlaceID
	Events             []string
	Story              ProgressState
	BlackedOut         bool

	LeadMoves         []Move
	LeadPP            []uint8
	RepelSteps        int
	Bag               []Item
	FieldCapabilities []FieldCapability
	RecentDialogue    []string
	History           []RoundRecord
	Failures          []Failure
	// CombatLossRecorded is typed evidence of an unresolved combat loss, so
	// economy policy never infers "boss failure" from objective names.
	CombatLossRecorded  bool                           `json:"combat_loss_recorded,omitempty"`
	ChallengeReadiness  []ChallengeReadiness           `json:"challenge_readiness,omitempty"`
	RecoveryCheckpoints []RecoveryCheckpointAssessment `json:"recovery_checkpoints,omitempty"`
	TrainingAreaChoices []TrainingAreaAssessment       `json:"training_areas,omitempty"`

	Round      int
	RoundsLeft int
	Intent     string
	IntentAge  int

	PokedexOwned []SpeciesID
	PokedexSeen  []SpeciesID
	Dex          DexCatalog       `json:"-"`
	Catalog      ObjectiveCatalog `json:"-"`

	WildGrass []WildSpecies
	HasGrass  bool
	Training  *TrainingEstimate `json:"training,omitempty"`
	MartStock []string
	// RestockStock lists what the nearest shop reachable by travel sells, for
	// travel-and-buy healing purchases (challengeHealingPurchase).
	RestockStock []string `json:"restock_stock,omitempty"`
	MapObjects   []MapObject

	Requirements   []Requirement
	RouteBlockages []RouteBlockage
	Unroutable     []string `json:"-"`

	// Services is present only when the process is configured with optional
	// runtime infrastructure. It is deliberately planner-visible so a model can
	// distinguish "trade evolutions need external help" from "this farm has a
	// virtual trader ready to service a link session".
	Services *RuntimeServices `json:"runtime_services,omitempty"`
}

// MapObject is one observable object on the current map. Item is a semantic
// item name when Kind is "item". Trainer fields represent semantic live state;
// the active game adapter owns how that state is decoded.
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

// semanticObservationAdapter owns the complete planner-facing observation for
// one game family. The profile supplies base semantic state and exact revision
// identity; the adapter enriches it with game-specific capabilities/catalogs.
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

// Observe preserves the historical panic-on-programmer-error compatibility
// wrapper. Production/replay/qualification entry points use ObserveChecked.
func Observe(m *emu.Emu, romData []byte) Observation {
	obs, err := ObserveChecked(m, romData)
	if err != nil {
		panic(err)
	}
	return obs
}

// ObserveChecked is deliberately game-agnostic. It identifies the exact game
// profile, selects that game's registered semantic observation adapter, and
// asks the adapter for the COMPLETE planner/runtime observation. Adding another
// game does not add a game-name switch to this function.
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
	if obs.GameID == "" {
		obs.GameID = profile.ID()
	}
	obs.Services = runtimeServicesFromEnvironment()
	return obs, nil
}
