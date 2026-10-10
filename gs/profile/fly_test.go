package profile

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/gs/data"
)

func TestGen2FlyTargetsStayInGameAdapter(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		for _, tc := range []struct {
			name, label string
		}{
			{"NEW_BARK_TOWN", "NEW BARK TOWN"},
			{"OLIVINE_CITY", "OLIVINE CITY"},
			{"CIANWOOD_CITY", "CIANWOOD CITY"},
			{"SILVER_CAVE_OUTSIDE", "MT.SILVER"},
			{"ROUTE_10_NORTH", "ROCK TUNNEL"},
			{"ROUTE_23", "INDIGO PLATEAU"},
		} {
			info, ok := data.MapByName(tc.name)
			if !ok {
				t.Fatalf("missing map %s", tc.name)
			}
			id := data.NativeMapID(info.Group, info.Number)
			got, ok := p.FlyDestinationLabel(id)
			if !ok || got != tc.label {
				t.Fatalf("%s FlyDestinationLabel(%s) = %q, %v; want %q", p.ID(), tc.name, got, ok, tc.label)
			}
		}
		if _, ok := p.FlyDestinationLabel(0xffff); ok {
			t.Fatal("unknown native map became a Fly landing")
		}
	}
}

func TestGen2FlyMenuOnlyDecodesWherePicker(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		var mem fakeMemory
		if got := p.DecodeFlyMenu(&mem); got.Visible {
			t.Fatalf("%s empty screen decoded Fly: %+v", p.ID(), got)
		}
		putGSScreenText(&mem, "PARTY PACK")
		if got := p.DecodeFlyMenu(&mem); got.Visible {
			t.Fatalf("%s party screen decoded Fly: %+v", p.ID(), got)
		}
		putGSScreenText(&mem, "Where? OLIVINE CITY")
		got := p.DecodeFlyMenu(&mem)
		if !got.Visible || !strings.HasPrefix(got.SelectedLabel, "OLIVINE CITY") {
			t.Fatalf("%s Fly picker = %+v", p.ID(), got)
		}
	}
}
