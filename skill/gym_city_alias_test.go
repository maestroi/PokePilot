package skill

import "testing"

func TestGymAtCityAliasesMatchInteriorChallenges(t *testing.T) {
	for _, tc := range []struct {
		city, gym uint8
		leader    string
	}{
		{city: 0x02, gym: 0x36, leader: "BROCK"},
		{city: 0x03, gym: 0x41, leader: "MISTY"},
	} {
		fromCity, ok := GymAt(tc.city)
		if !ok {
			t.Fatalf("city %#04x has no gym challenge", tc.city)
		}
		inside, ok := GymAt(tc.gym)
		if !ok {
			t.Fatalf("gym %#04x has no gym challenge", tc.gym)
		}
		if fromCity != inside || fromCity.Leader != tc.leader {
			t.Fatalf("city %#04x challenge = %+v, interior = %+v, want %s alias", tc.city, fromCity, inside, tc.leader)
		}
	}
}
