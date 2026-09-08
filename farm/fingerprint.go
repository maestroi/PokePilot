package farm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	// FailureIdentityVersion is the canonical schema version hashed into every
	// structured failure fingerprint. Changing fingerprint semantics requires a
	// new version rather than silently changing the meaning of an old digest.
	FailureIdentityVersion = 1
	failureDetailPrefix    = "failure-id:"
)

// FailureObjective is the portable, planner-selected operation that failed.
// It deliberately excludes presentation-only fields such as notes and intent.
type FailureObjective struct {
	Kind     string `json:"kind"`
	Place    string `json:"place,omitempty"`
	X        uint8  `json:"x,omitempty"`
	Y        uint8  `json:"y,omitempty"`
	Starter  string `json:"starter,omitempty"`
	Progress string `json:"progress,omitempty"`
	Level    uint8  `json:"level,omitempty"`
	Species  string `json:"species,omitempty"`
	Item     string `json:"item,omitempty"`
	Slot     int    `json:"slot,omitempty"`
	Qty      int    `json:"qty,omitempty"`
	Flee     bool   `json:"flee,omitempty"`
}

type FailurePartyMember struct {
	Species string `json:"species"`
	Level   uint8  `json:"level"`
	HP      uint16 `json:"hp"`
	MaxHP   uint16 `json:"max_hp"`
	Status  string `json:"status,omitempty"`
}

type FailureInventoryItem struct {
	ID       string `json:"id"`
	Quantity int    `json:"quantity"`
}

type FailureCapability struct {
	ID         string `json:"id"`
	BadgeOwned bool   `json:"badge_owned,omitempty"`
	HMOwned    bool   `json:"hm_owned,omitempty"`
	Learned    bool   `json:"learned,omitempty"`
	Usable     bool   `json:"usable,omitempty"`
}

type FailureProgressFact struct {
	ID       string `json:"id"`
	Complete bool   `json:"complete,omitempty"`
	Value    int    `json:"value,omitempty"`
}

// FailureState is the relevant semantic world state around one objective
// failure. Raw RAM addresses, Red map bytes and diagnostic prose never enter
// the canonical identity.
type FailureState struct {
	Location     string                 `json:"location"`
	X            uint8                  `json:"x"`
	Y            uint8                  `json:"y"`
	Controllable bool                   `json:"controllable"`
	InBattle     bool                   `json:"in_battle"`
	Money        uint32                 `json:"money"`
	Party        []FailurePartyMember   `json:"party,omitempty"`
	Inventory    []FailureInventoryItem `json:"inventory,omitempty"`
	Badges       []string               `json:"badges,omitempty"`
	Capabilities []FailureCapability    `json:"capabilities,omitempty"`
	Progress     []FailureProgressFact  `json:"progress,omitempty"`
}

// FailureIdentity is the logical defect identity. Build/revision is
// intentionally occurrence metadata rather than hash input: the same defect on
// a newer build must retain the same fingerprint so a fixed issue can be
// recognized as a regression instead of becoming unrelated work.
type FailureIdentity struct {
	Version      int              `json:"version"`
	Game         string           `json:"game"`
	Adapter      string           `json:"adapter"`
	Objective    FailureObjective `json:"objective"`
	Outcome      string           `json:"outcome"`
	Cause        string           `json:"cause"`
	CauseContext []string         `json:"cause_context,omitempty"`
	Initial      FailureState     `json:"initial"`
	Final        FailureState     `json:"final"`
}

// FailureOccurrence is one sighting of a logical failure. Fingerprint/Key are
// redundant on purpose: persisted evidence is self-checking, while Build,
// checkpoint and diagnostic text remain occurrence facts and never influence
// deduplication.
type FailureOccurrence struct {
	Key         string          `json:"key"`
	Fingerprint string          `json:"fingerprint"`
	Identity    FailureIdentity `json:"identity"`
	Build       string          `json:"build,omitempty"`
	Round       int             `json:"round,omitempty"`
	Checkpoint  string          `json:"checkpoint,omitempty"`
	Diagnostic  string          `json:"diagnostic,omitempty"`
	ObservedAt  time.Time       `json:"observed_at"`
}

// FingerprintFailureIdentity returns the stable short key and full SHA-256
// fingerprint for identity. Canonicalization is structural: order-insensitive
// semantic sets are sorted, while party order is preserved because lead/slot
// identity is gameplay-relevant.
func FingerprintFailureIdentity(identity FailureIdentity) (key, fingerprint string, err error) {
	canonical := canonicalFailureIdentity(identity)
	if canonical.Version == 0 {
		canonical.Version = FailureIdentityVersion
	}
	if canonical.Version != FailureIdentityVersion {
		return "", "", fmt.Errorf("farm: failure identity version %d, want %d", canonical.Version, FailureIdentityVersion)
	}
	if canonical.Game == "" || canonical.Adapter == "" || canonical.Objective.Kind == "" || canonical.Outcome == "" || canonical.Cause == "" {
		return "", "", fmt.Errorf("farm: incomplete failure identity")
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", "", fmt.Errorf("farm: encode failure identity: %w", err)
	}
	sum := sha256.Sum256(data)
	h := hex.EncodeToString(sum[:])
	return h[:16], "sha256:" + h, nil
}

func NewFailureOccurrence(identity FailureIdentity, build string, round int, checkpoint, diagnostic string, observedAt time.Time) (FailureOccurrence, error) {
	identity = canonicalFailureIdentity(identity)
	if identity.Version == 0 {
		identity.Version = FailureIdentityVersion
	}
	key, fingerprint, err := FingerprintFailureIdentity(identity)
	if err != nil {
		return FailureOccurrence{}, err
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	return FailureOccurrence{
		Key:         key,
		Fingerprint: fingerprint,
		Identity:    identity,
		Build:       strings.TrimSpace(build),
		Round:       round,
		Checkpoint:  strings.TrimSpace(checkpoint),
		Diagnostic:  strings.TrimSpace(diagnostic),
		ObservedAt:  observedAt.UTC(),
	}, nil
}

func ValidateFailureOccurrence(o FailureOccurrence) error {
	key, fingerprint, err := FingerprintFailureIdentity(o.Identity)
	if err != nil {
		return err
	}
	if o.Key != key || o.Fingerprint != fingerprint {
		return fmt.Errorf("farm: failure occurrence fingerprint mismatch")
	}
	return nil
}

// FailureDetailMarker is a compact, stable top-level run detail for wall
// grouping. The digest is encoded with letters a-p so the legacy triage
// number normalizer cannot mutate it. Human diagnostics stay in structured
// occurrence evidence instead of becoming identity syntax.
func FailureDetailMarker(o FailureOccurrence) string {
	hexDigest := strings.TrimPrefix(o.Fingerprint, "sha256:")
	if len(hexDigest) != 64 {
		return ""
	}
	var b strings.Builder
	b.Grow(len(failureDetailPrefix) + 64 + 48)
	b.WriteString(failureDetailPrefix)
	for _, r := range hexDigest {
		var n byte
		switch {
		case r >= '0' && r <= '9':
			n = byte(r - '0')
		case r >= 'a' && r <= 'f':
			n = byte(r-'a') + 10
		default:
			return ""
		}
		b.WriteByte('a' + n)
	}
	id := o.Identity
	fmt.Fprintf(&b, " %s %s %s", id.Objective.Kind, id.Outcome, id.Cause)
	if id.Objective.Place != "" {
		b.WriteByte(' ')
		b.WriteString(id.Objective.Place)
	}
	return b.String()
}

// ParseFailureDetailMarker recovers a canonical fingerprint from the stable
// top-level detail marker. Extra stable display words after the digest are
// ignored.
func ParseFailureDetailMarker(detail string) (key, fingerprint string, ok bool) {
	detail = strings.TrimSpace(detail)
	if !strings.HasPrefix(detail, failureDetailPrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(detail, failureDetailPrefix)
	if i := strings.IndexByte(rest, ' '); i >= 0 {
		rest = rest[:i]
	}
	if len(rest) != 64 {
		return "", "", false
	}
	var b strings.Builder
	b.Grow(64)
	const hexChars = "0123456789abcdef"
	for _, r := range rest {
		if r < 'a' || r > 'p' {
			return "", "", false
		}
		b.WriteByte(hexChars[int(r-'a')])
	}
	h := b.String()
	return h[:16], "sha256:" + h, true
}

func canonicalFailureIdentity(in FailureIdentity) FailureIdentity {
	out := in
	out.Version = in.Version
	out.Game = canonicalFailureString(in.Game)
	out.Adapter = canonicalFailureString(in.Adapter)
	out.Objective.Kind = canonicalFailureString(in.Objective.Kind)
	out.Objective.Place = canonicalFailureString(in.Objective.Place)
	out.Objective.Starter = canonicalFailureString(in.Objective.Starter)
	out.Objective.Progress = canonicalFailureString(in.Objective.Progress)
	out.Objective.Species = canonicalFailureString(in.Objective.Species)
	out.Objective.Item = canonicalFailureString(in.Objective.Item)
	out.Outcome = canonicalFailureString(in.Outcome)
	out.Cause = canonicalFailureString(in.Cause)
	out.CauseContext = append([]string(nil), in.CauseContext...)
	for i := range out.CauseContext {
		out.CauseContext[i] = canonicalFailureString(out.CauseContext[i])
	}
	sort.Strings(out.CauseContext)
	out.Initial = canonicalFailureState(in.Initial)
	out.Final = canonicalFailureState(in.Final)
	return out
}

func canonicalFailureState(in FailureState) FailureState {
	out := in
	out.Location = canonicalFailureString(in.Location)
	out.Party = append([]FailurePartyMember(nil), in.Party...)
	for i := range out.Party {
		out.Party[i].Species = canonicalFailureString(out.Party[i].Species)
		out.Party[i].Status = canonicalFailureString(out.Party[i].Status)
	}
	out.Inventory = append([]FailureInventoryItem(nil), in.Inventory...)
	for i := range out.Inventory {
		out.Inventory[i].ID = canonicalFailureString(out.Inventory[i].ID)
	}
	sort.Slice(out.Inventory, func(i, j int) bool {
		if out.Inventory[i].ID != out.Inventory[j].ID {
			return out.Inventory[i].ID < out.Inventory[j].ID
		}
		return out.Inventory[i].Quantity < out.Inventory[j].Quantity
	})
	out.Badges = append([]string(nil), in.Badges...)
	for i := range out.Badges {
		out.Badges[i] = canonicalFailureString(out.Badges[i])
	}
	sort.Strings(out.Badges)
	out.Capabilities = append([]FailureCapability(nil), in.Capabilities...)
	for i := range out.Capabilities {
		out.Capabilities[i].ID = canonicalFailureString(out.Capabilities[i].ID)
	}
	sort.Slice(out.Capabilities, func(i, j int) bool { return out.Capabilities[i].ID < out.Capabilities[j].ID })
	out.Progress = append([]FailureProgressFact(nil), in.Progress...)
	for i := range out.Progress {
		out.Progress[i].ID = canonicalFailureString(out.Progress[i].ID)
	}
	sort.Slice(out.Progress, func(i, j int) bool { return out.Progress[i].ID < out.Progress[j].ID })
	return out
}

func canonicalFailureString(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
