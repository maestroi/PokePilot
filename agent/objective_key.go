package agent

import (
	"encoding/json"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

// ObjectiveKey is the canonical semantic identity of an objective. It includes
// every field that can change execution semantics or durable local identity and
// deliberately excludes presentation-only Note text. Location scopes
// coordinate-local interactions without leaking a native map ID. Intent is
// included because several adapter objectives (for example dex evolution/static
// acquisition) use it to select a different deterministic execution path.
type ObjectiveKey struct {
	Kind     Kind       `json:"kind"`
	Place    PlaceID    `json:"place,omitempty"`
	Location LocationID `json:"location,omitempty"`
	X        uint8      `json:"x,omitempty"`
	Y        uint8      `json:"y,omitempty"`
	Starter  uint8      `json:"starter,omitempty"`
	Progress ProgressID `json:"progress,omitempty"`
	Level    uint8      `json:"level,omitempty"`
	Species  SpeciesID  `json:"species,omitempty"`
	Item     ItemID     `json:"item,omitempty"`
	Slot     int        `json:"slot,omitempty"`
	Qty      int        `json:"qty,omitempty"`
	Flee     bool       `json:"flee,omitempty"`
	Intent   string     `json:"intent,omitempty"`
}

func (o Objective) Key() ObjectiveKey {
	location, x, y := o.Location, o.X, o.Y
	// Travel coordinates are adapter-derived execution metadata. The semantic
	// Place is the durable identity: preserving native/stand coordinates in a
	// checkpoint key would make the same journey forget its history whenever a
	// profile improves its chosen landing tile. Resumed travel re-resolves the
	// active catalog by Place before execution.
	if o.Kind == KindGoTo {
		location, x, y = "", 0, 0
	}
	return ObjectiveKey{
		Kind:     o.Kind,
		Place:    o.Place,
		Location: location,
		X:        x,
		Y:        y,
		Starter:  uint8(o.Starter),
		Progress: o.Progress,
		Level:    o.Level,
		Species:  o.Species,
		Item:     o.Item,
		Slot:     o.Slot,
		Qty:      o.Qty,
		Flee:     o.Flee,
		Intent:   o.Intent,
	}
}

func (k ObjectiveKey) Objective() Objective {
	return Objective{
		Kind:     k.Kind,
		Place:    k.Place,
		Location: k.Location,
		X:        k.X,
		Y:        k.Y,
		Starter:  skill.Starter(k.Starter),
		Progress: k.Progress,
		Level:    k.Level,
		Species:  k.Species,
		Item:     k.Item,
		Slot:     k.Slot,
		Qty:      k.Qty,
		Flee:     k.Flee,
		Intent:   k.Intent,
	}
}

// ID is the stable map-key form used by in-memory durable bookkeeping. The
// persisted checkpoint representation stores ObjectiveKey structurally; this
// compact form only exists because Go maps cannot use a struct key in the
// historical public Knowledge shape without breaking callers in one release.
func (k ObjectiveKey) ID() string {
	data, _ := json.Marshal(k)
	return string(data)
}

func parseObjectiveKeyID(id string) (ObjectiveKey, bool) {
	var key ObjectiveKey
	if id == "" || json.Unmarshal([]byte(id), &key) != nil {
		return ObjectiveKey{}, false
	}
	return key, true
}

func resolveObjectiveKey(offered []Objective, key ObjectiveKey) (Objective, bool) {
	for _, o := range offered {
		if o.Key() == key {
			return o, true
		}
	}
	return Objective{}, false
}

func objectiveStorageKey(o Objective) string { return o.Key().ID() }

const (
	failureModeGymLoss     = "gym_loss"
	failureModeGymRetry    = "gym_retry"
	failureModeTrainerLoss = "trainer_loss"
)

func failureStorageKey(key ObjectiveKey, mode string) string {
	if mode == "" {
		return key.ID()
	}
	return mode + ":" + key.ID()
}

func parseFailureStorageKey(id string) (ObjectiveKey, string, bool) {
	if key, ok := parseObjectiveKeyID(id); ok {
		return key, "", true
	}
	for _, mode := range []string{failureModeGymLoss, failureModeGymRetry, failureModeTrainerLoss} {
		prefix := mode + ":"
		if strings.HasPrefix(id, prefix) {
			key, ok := parseObjectiveKeyID(strings.TrimPrefix(id, prefix))
			return key, mode, ok
		}
	}
	return ObjectiveKey{}, "", false
}
