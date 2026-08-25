package state

import (
	"fmt"
	"math/bits"

	"github.com/maestroi/pokepilot/red/sym"
)

// Badge is one of the eight Kanto badges.
type Badge uint8

const (
	BadgeBoulder Badge = iota
	BadgeCascade
	BadgeThunder
	BadgeRainbow
	BadgeSoul
	BadgeMarsh
	BadgeVolcano
	BadgeEarth
)

var badgeNames = [...]string{
	"Boulder", "Cascade", "Thunder", "Rainbow",
	"Soul", "Marsh", "Volcano", "Earth",
}

// String renders the badge name; out-of-range values render as "unknown(N)".
func (b Badge) String() string {
	if int(b) < len(badgeNames) {
		return badgeNames[b]
	}
	return fmt.Sprintf("unknown(%d)", uint8(b))
}

// ProgressState is the decoded badge progress.
type ProgressState struct {
	Badges     uint8 // raw bitfield
	BadgeCount int
}

// Has reports whether badge b is set in the bitfield.
func (p ProgressState) Has(b Badge) bool {
	return p.Badges&(1<<uint8(b)) != 0
}

// DecodeProgress reads the badge bitfield from a RAM snapshot. Bit 0 is the
// Boulder Badge through bit 7 for the Earth Badge.
func DecodeProgress(m *Mem) ProgressState {
	badges := m.U8(sym.ObtainedBadges)
	return ProgressState{Badges: badges, BadgeCount: bits.OnesCount8(badges)}
}

// Event identifies a story flag by its bit index in wEventFlags.
type Event uint16

const (
	EventFollowedOakIntoLab    Event = 0
	EventOakAskedToChooseMon   Event = 33
	EventGotStarter            Event = 34
	EventBattledRivalInOaksLab Event = 35
	EventGotPokeballsFromOak   Event = 36
	EventOakAppearedInPallet   Event = 38
)

var eventNames = map[Event]string{
	EventFollowedOakIntoLab:    "FollowedOakIntoLab",
	EventOakAskedToChooseMon:   "OakAskedToChooseMon",
	EventGotStarter:            "GotStarter",
	EventBattledRivalInOaksLab: "BattledRivalInOaksLab",
	EventGotPokeballsFromOak:   "GotPokeballsFromOak",
	EventOakAppearedInPallet:   "OakAppearedInPallet",
}

// String renders the event name; unnamed indices render as "unknown(N)".
func (e Event) String() string {
	if name, ok := eventNames[e]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", uint16(e))
}

// HasEvent reports whether the story event flag is set. Bit N of wEventFlags
// lives in byte 0xD747+N/8 at bit N%8.
func HasEvent(m *Mem, e Event) bool {
	b := m.U8(sym.EventFlags + uint16(e)/8)
	return b&(1<<(uint16(e)%8)) != 0
}
