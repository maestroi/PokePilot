// Package renderstate defines the game-agnostic semantic state consumed by
// spectator renderers. It is transport-neutral: live spectator, operator live
// view, and replay may all serialize the same contract without exposing RAM,
// ROM, or emulator-specific structures to frontends.
package renderstate

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/maestroi/pokepilot/game"
)

// SchemaVersion is the current RenderState wire schema. Additive optional
// fields and new string enum values remain within a schema version; bump this
// only for a breaking wire change.
const SchemaVersion = 1

// Scene identifies the high-level presentation surface. Values are open-ended
// strings so an older consumer can retain an unknown future scene without
// needing game-specific fallback logic.
type Scene string

const (
	SceneUnknown    Scene = "unknown"
	SceneOverworld  Scene = "overworld"
	SceneBattle     Scene = "battle"
	SceneMenu       Scene = "menu"
	SceneDialogue   Scene = "dialogue"
	SceneTransition Scene = "transition"
)

// Direction is a semantic facing direction in world coordinates.
type Direction string

const (
	DirectionUnknown Direction = "unknown"
	DirectionUp      Direction = "up"
	DirectionDown    Direction = "down"
	DirectionLeft    Direction = "left"
	DirectionRight   Direction = "right"
)

// MovementKind describes authoritative movement intent/state. Renderers may
// interpolate between From and To, but interpolated positions must never feed
// back into gameplay state.
type MovementKind string

const (
	MovementUnknown MovementKind = "unknown"
	MovementIdle    MovementKind = "idle"
	MovementWalk    MovementKind = "walk"
	MovementRun     MovementKind = "run"
	MovementSlide   MovementKind = "slide"
	MovementJump    MovementKind = "jump"
)

// EntityKind is deliberately extensible. The built-in values cover common
// overworld concepts without requiring every game to expose the same set.
type EntityKind string

const (
	EntityUnknown EntityKind = "unknown"
	EntityPlayer  EntityKind = "player"
	EntityNPC     EntityKind = "npc"
	EntityTrainer EntityKind = "trainer"
	EntityItem    EntityKind = "item"
	EntityObject  EntityKind = "object"
)

// LayerKind describes a semantic map layer. Unknown future layer values are
// valid and may be ignored by consumers that do not understand them.
type LayerKind string

const (
	LayerTerrain LayerKind = "terrain"
	LayerObjects LayerKind = "objects"
	LayerOverlay LayerKind = "overlay"
)

// TileKind names semantic terrain/object meanings rather than native tile or
// block numbers. Adapter-specific extraction maps native data onto these or
// additional future semantic values.
type TileKind string

const (
	TileUnknown TileKind = "unknown"
	TileGrass   TileKind = "grass"
	TilePath    TileKind = "path"
	TileWater   TileKind = "water"
	TileTree    TileKind = "tree"
	TileLedge   TileKind = "ledge"
	TileWall    TileKind = "wall"
	TileDoor    TileKind = "door"
	TileFloor   TileKind = "floor"
	TileWarp    TileKind = "warp"
)

// Capability identifies an optional semantic surface supplied by a producer.
// Consumers should test capabilities rather than infer support from a game ID.
type Capability string

const (
	CapabilityMap        Capability = "map"
	CapabilityCamera     Capability = "camera"
	CapabilityPlayer     Capability = "player"
	CapabilityEntities   Capability = "entities"
	CapabilityLayers     Capability = "layers"
	CapabilityDialogue   Capability = "dialogue"
	CapabilityMenu       Capability = "menu"
	CapabilityBattle     Capability = "battle"
	CapabilityTransition Capability = "transition"
	CapabilityEffects    Capability = "effects"
)

// GameRef identifies which adapter/revision produced a state without exposing
// native ROM tables or memory layout details.
type GameRef struct {
	ID       game.GameID     `json:"id"`
	Revision game.RevisionID `json:"revision"`
}

// Clock carries authoritative emulator time plus an optional capture timestamp.
// CapturedAtUnixMS is presentation metadata, not a gameplay time source.
type Clock struct {
	Frame            uint64 `json:"frame"`
	Cycle            uint64 `json:"cycle,omitempty"`
	CapturedAtUnixMS int64  `json:"captured_at_unix_ms,omitempty"`
}

// Position is expressed in semantic world tile coordinates. Sub-tile motion is
// represented separately by MovementState.
type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// CameraState is expressed in world tile units so themes can choose their own
// pixel scale.
type CameraState struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
}

// MovementState records an authoritative movement segment. Progress is in the
// inclusive range [0,1] when known; zero with Kind=unknown means unavailable.
type MovementState struct {
	Kind     MovementKind `json:"kind"`
	From     Position     `json:"from"`
	To       Position     `json:"to"`
	Progress float64      `json:"progress,omitempty"`
}

// MapState identifies the current semantic map and optional camera/world size.
type MapState struct {
	ID     game.PlaceID `json:"id"`
	Name   string       `json:"name,omitempty"`
	Width  int          `json:"width,omitempty"`
	Height int          `json:"height,omitempty"`
	Camera *CameraState `json:"camera,omitempty"`
}

// ActorState describes the player or a visible world entity. Appearance is a
// semantic asset key, not a ROM sprite/picture number.
type ActorState struct {
	ID         string         `json:"id,omitempty"`
	Kind       EntityKind     `json:"kind"`
	Label      string         `json:"label,omitempty"`
	Appearance string         `json:"appearance,omitempty"`
	Position   Position       `json:"position"`
	Facing     Direction      `json:"facing,omitempty"`
	Movement   *MovementState `json:"movement,omitempty"`
}

// TileCell is one semantic cell in a row-major TileLayer grid. Variant and
// Tags are theme/render hints with no gameplay authority.
type TileCell struct {
	Kind    TileKind `json:"kind"`
	Variant string   `json:"variant,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// TileLayer is a row-major semantic grid anchored at Origin.
type TileLayer struct {
	ID     string     `json:"id"`
	Kind   LayerKind  `json:"kind"`
	Origin Position   `json:"origin"`
	Width  int        `json:"width"`
	Height int        `json:"height"`
	Cells  []TileCell `json:"cells"`
}

// MenuEntry is a semantic menu option. ID is stable machine meaning where an
// adapter has one; Label is presentation text when available.
type MenuEntry struct {
	ID       string `json:"id,omitempty"`
	Label    string `json:"label,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

// MenuState represents a currently visible menu surface when a producer can
// decode it. Cursor is nil when the cursor position is unavailable.
type MenuState struct {
	ID      string      `json:"id,omitempty"`
	Title   string      `json:"title,omitempty"`
	Cursor  *int        `json:"cursor,omitempty"`
	Entries []MenuEntry `json:"entries,omitempty"`
}

// DialogueState represents visible dialogue without exposing native text-box
// addresses or script state.
type DialogueState struct {
	Speaker string      `json:"speaker,omitempty"`
	Text    string      `json:"text,omitempty"`
	Choices []MenuEntry `json:"choices,omitempty"`
	Cursor  *int        `json:"cursor,omitempty"`
}

// BattleActor is the minimal cross-game presentation state for one participant.
// Producers may leave unsupported fields empty.
type BattleActor struct {
	ID         string `json:"id,omitempty"`
	Role       string `json:"role,omitempty"`
	Name       string `json:"name,omitempty"`
	Appearance string `json:"appearance,omitempty"`
	Level      int    `json:"level,omitempty"`
	HP         int    `json:"hp,omitempty"`
	MaxHP      int    `json:"max_hp,omitempty"`
	Status     string `json:"status,omitempty"`
	Active     bool   `json:"active,omitempty"`
	Defeated   bool   `json:"defeated,omitempty"`
}

// BattleMove is one presentation-level move slot for the active player actor.
// ID is semantic and stable when the producer has one; native ROM indexes stay
// adapter-owned.
type BattleMove struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	PP       int    `json:"pp,omitempty"`
	MaxPP    int    `json:"max_pp,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

// BattleState is intentionally presentation-oriented rather than a battle
// controller contract. It describes what a spectator may render, never which
// action the game should take.
type BattleState struct {
	Kind   string        `json:"kind,omitempty"`
	Phase  string        `json:"phase,omitempty"`
	Turn   int           `json:"turn,omitempty"`
	Actors []BattleActor `json:"actors,omitempty"`
	Moves  []BattleMove  `json:"moves,omitempty"`
}

// TransitionState describes an authoritative scene/map transition.
type TransitionState struct {
	Kind     string       `json:"kind,omitempty"`
	FromMap  game.PlaceID `json:"from_map,omitempty"`
	ToMap    game.PlaceID `json:"to_map,omitempty"`
	Progress float64      `json:"progress,omitempty"`
}

// EffectState is a transient semantic effect. Data is optional string metadata
// for render-only hints and must not become gameplay policy.
type EffectState struct {
	Kind           string            `json:"kind"`
	EntityID       string            `json:"entity_id,omitempty"`
	Position       *Position         `json:"position,omitempty"`
	StartedFrame   uint64            `json:"started_frame,omitempty"`
	DurationFrames uint64            `json:"duration_frames,omitempty"`
	Data           map[string]string `json:"data,omitempty"`
}

// RenderState is the stable semantic wire contract shared by live spectator,
// operator live view, and replay. Optional sections may be absent; consumers
// should use Capabilities to discover producer support and ignore unknown JSON
// fields or open string values they do not understand.
type RenderState struct {
	SchemaVersion int          `json:"schema_version"`
	Game          GameRef      `json:"game"`
	Clock         Clock        `json:"clock"`
	Scene         Scene        `json:"scene"`
	Capabilities  []Capability `json:"capabilities,omitempty"`

	Map        *MapState        `json:"map,omitempty"`
	Player     *ActorState      `json:"player,omitempty"`
	Entities   []ActorState     `json:"entities,omitempty"`
	Layers     []TileLayer      `json:"layers,omitempty"`
	Dialogue   *DialogueState   `json:"dialogue,omitempty"`
	Menu       *MenuState       `json:"menu,omitempty"`
	Battle     *BattleState     `json:"battle,omitempty"`
	Transition *TransitionState `json:"transition,omitempty"`
	Effects    []EffectState    `json:"effects,omitempty"`
}

// FrameMeta supplies transport/emulator metadata that does not belong to
// game.ProfileObservation itself.
type FrameMeta struct {
	GameID           game.GameID
	Revision         game.RevisionID
	Frame            uint64
	Cycle            uint64
	CapturedAtUnixMS int64
}

// ProfileSource is the narrow profile surface needed to produce baseline
// RenderState. Every game.GameProfile satisfies it.
type ProfileSource interface {
	ID() game.GameID
	Revision() game.RevisionID
	DecodeObservation(game.MemoryReader, []byte) (game.ProfileObservation, error)
}

// FromProfile decodes one game-owned semantic observation and converts it to
// RenderState. The profile identity is authoritative; callers cannot
// accidentally label one game's observation as another game.
func FromProfile(profile ProfileSource, reader game.MemoryReader, romData []byte, meta FrameMeta) (RenderState, error) {
	if profile == nil {
		return RenderState{}, fmt.Errorf("renderstate: profile is required")
	}
	obs, err := profile.DecodeObservation(reader, romData)
	if err != nil {
		return RenderState{}, fmt.Errorf("renderstate: decode profile observation: %w", err)
	}
	meta.GameID = profile.ID()
	meta.Revision = profile.Revision()
	state := FromProfileObservation(meta, obs)
	if err := Validate(state); err != nil {
		return RenderState{}, err
	}
	return state, nil
}

// FromProfileObservation creates the baseline semantic state available from
// every GameProfile today. Rich adapter-owned world/entity extraction can
// enrich this state later without changing the transport or frontend contract.
func FromProfileObservation(meta FrameMeta, obs game.ProfileObservation) RenderState {
	scene := SceneOverworld
	caps := []Capability{CapabilityMap, CapabilityPlayer}
	var battle *BattleState
	if obs.InBattle {
		scene = SceneBattle
		caps = append(caps, CapabilityBattle)
		battle = &BattleState{}
	}

	state := RenderState{
		SchemaVersion: SchemaVersion,
		Game: GameRef{
			ID:       meta.GameID,
			Revision: meta.Revision,
		},
		Clock: Clock{
			Frame:            meta.Frame,
			Cycle:            meta.Cycle,
			CapturedAtUnixMS: meta.CapturedAtUnixMS,
		},
		Scene:        scene,
		Capabilities: caps,
		Map: &MapState{
			ID:   obs.Location,
			Name: obs.MapName,
		},
		Player: &ActorState{
			ID:       "player",
			Kind:     EntityPlayer,
			Position: Position{X: int(obs.X), Y: int(obs.Y)},
			Facing:   Direction(strings.ToLower(strings.TrimSpace(obs.Facing))),
		},
		Battle: battle,
	}
	if state.Player.Facing == "" {
		state.Player.Facing = DirectionUnknown
	}
	return state
}

// HasCapability reports whether the producer advertised capability.
func (s RenderState) HasCapability(capability Capability) bool {
	for _, candidate := range s.Capabilities {
		if candidate == capability {
			return true
		}
	}
	return false
}

// Validate checks only cross-game wire invariants. It intentionally does not
// reject unknown scene/entity/tile/capability values: open string vocabularies
// are how additive protocol evolution degrades gracefully.
func Validate(s RenderState) error {
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("renderstate: unsupported schema %d (want %d)", s.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(string(s.Game.ID)) == "" {
		return fmt.Errorf("renderstate: game id is required")
	}
	if strings.TrimSpace(string(s.Game.Revision)) == "" {
		return fmt.Errorf("renderstate: game revision is required")
	}
	if strings.TrimSpace(string(s.Scene)) == "" {
		return fmt.Errorf("renderstate: scene is required")
	}
	if s.Map != nil && (s.Map.Width < 0 || s.Map.Height < 0) {
		return fmt.Errorf("renderstate: map dimensions cannot be negative")
	}
	seenCaps := make(map[Capability]struct{}, len(s.Capabilities))
	for _, capability := range s.Capabilities {
		if strings.TrimSpace(string(capability)) == "" {
			return fmt.Errorf("renderstate: capability cannot be empty")
		}
		if _, duplicate := seenCaps[capability]; duplicate {
			return fmt.Errorf("renderstate: duplicate capability %q", capability)
		}
		seenCaps[capability] = struct{}{}
	}
	for _, layer := range s.Layers {
		if strings.TrimSpace(layer.ID) == "" {
			return fmt.Errorf("renderstate: layer id is required")
		}
		if layer.Width < 0 || layer.Height < 0 {
			return fmt.Errorf("renderstate: layer %q dimensions cannot be negative", layer.ID)
		}
		if want := layer.Width * layer.Height; len(layer.Cells) != want {
			return fmt.Errorf("renderstate: layer %q has %d cells, want %d", layer.ID, len(layer.Cells), want)
		}
	}
	return nil
}

// WriteJSON validates and serializes one state. The transport remains outside
// this package so live HTTP, WebSocket/SSE, files, and replay can share it.
func WriteJSON(w io.Writer, state RenderState) error {
	if err := Validate(state); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	return enc.Encode(state)
}

// ReadJSON decodes one state. encoding/json ignores unknown object fields, so
// additive fields from a newer producer do not break a same-version consumer.
func ReadJSON(r io.Reader) (RenderState, error) {
	var state RenderState
	if err := json.NewDecoder(r).Decode(&state); err != nil {
		return RenderState{}, fmt.Errorf("renderstate: decode: %w", err)
	}
	if err := Validate(state); err != nil {
		return RenderState{}, err
	}
	return state, nil
}
