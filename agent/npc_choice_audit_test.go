package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestAuditedCustomChoiceActorsRejectGenericTalk(t *testing.T) {
	for _, actor := range redCustomChoiceActors {
		o := Objective{Kind: KindTalk, X: actor.x, Y: actor.y}
		err := validateRedTalkObjective(nil, actor.mapID, o)
		if err == nil {
			t.Fatalf("generic Talk accepted audited choice actor on map %#02x at (%d,%d)", actor.mapID, actor.x, actor.y)
		}
		if !errors.Is(err, skill.ErrNoDialogue) {
			t.Fatalf("choice actor on map %#02x at (%d,%d) error = %v, want recoverable blocked classification", actor.mapID, actor.x, actor.y, err)
		}
	}
}

func TestCustomChoiceAuditDoesNotSuppressNearbyOrdinaryNPC(t *testing.T) {
	cases := []struct {
		name  string
		mapID uint8
		x     uint8
		y     uint8
	}{
		{name: "Lavender cooltrainer", mapID: 0x04, x: 9, y: 10},
		{name: "Lavender super nerd", mapID: 0x04, x: 8, y: 7},
		{name: "Cinnabar fossil room empty coordinate", mapID: 0xAA, x: 6, y: 2},
		{name: "Name Rater house empty coordinate", mapID: 0xE5, x: 4, y: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if redChoiceInteractionActor(tc.mapID, tc.x, tc.y) {
				t.Fatalf("ordinary/non-actor coordinate on map %#02x at (%d,%d) was classified as a choice actor", tc.mapID, tc.x, tc.y)
			}
		})
	}
}
