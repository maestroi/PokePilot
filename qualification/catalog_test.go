package qualification

import (
	"strings"
	"testing"
)

func TestCatalogIDsAreUniqueAndSafe(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Catalog() {
		if c.ID == "" {
			t.Fatal("catalog contains empty id")
		}
		if strings.ContainsAny(c.ID, " /\\\t\n") {
			t.Fatalf("case id %q is not artifact/path safe", c.ID)
		}
		if seen[c.ID] {
			t.Fatalf("duplicate case id %q", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestCatalogContainsQualificationRoadmap(t *testing.T) {
	want := map[string]int{
		"opening-brock":              0,
		"mt-moon-cerulean":           0,
		"misty":                      0,
		"rocket-hideout":             0,
		"pokemon-tower":              0,
		"fuchsia-koga-surf-strength": 0,
		"silph-sabrina":              0,
		"cinnabar-blaine":            0,
		"viridian-giovanni":          0,
		"victory-road-indigo":        0,
		"elite-four-champion":        0,
		"fresh-hall-of-fame":         0,
	}
	got := map[string]Case{}
	for _, c := range Catalog() {
		got[c.ID] = c
	}
	for id, blocker := range want {
		c, ok := got[id]
		if !ok {
			t.Errorf("missing qualification case %q", id)
			continue
		}
		if c.BlockedBy != blocker {
			t.Errorf("case %q blocked_by=%d, want %d", id, c.BlockedBy, blocker)
		}
	}
}

func TestSelectProfiles(t *testing.T) {
	tests := []struct {
		profile string
		want    []string
	}{
		{profile: "skills", want: []string{"rom-short"}},
		{profile: "milestones", want: []string{"opening-brock", "mt-moon-cerulean", "misty", "rocket-hideout", "pokemon-tower", "fuchsia-koga-surf-strength", "silph-sabrina", "cinnabar-blaine", "viridian-giovanni", "victory-road-indigo", "elite-four-champion"}},
		{profile: "full", want: []string{"fresh-hall-of-fame"}},
		{profile: "", want: []string{"opening-brock", "mt-moon-cerulean", "misty", "rocket-hideout", "pokemon-tower", "fuchsia-koga-surf-strength", "silph-sabrina", "cinnabar-blaine", "viridian-giovanni", "victory-road-indigo", "elite-four-champion"}},
	}
	for _, tc := range tests {
		t.Run(tc.profile, func(t *testing.T) {
			cases, err := Select(tc.profile, "")
			if err != nil {
				t.Fatal(err)
			}
			if len(cases) != len(tc.want) {
				t.Fatalf("cases=%v, want ids %v", caseIDs(cases), tc.want)
			}
			for i := range tc.want {
				if cases[i].ID != tc.want[i] {
					t.Fatalf("case %d=%q, want %q; all=%v", i, cases[i].ID, tc.want[i], caseIDs(cases))
				}
			}
		})
	}
}

func TestSelectAllIncludesEveryLandedMilestone(t *testing.T) {
	cases, err := Select("all", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if !c.Available {
			t.Fatalf("Select returned unavailable case %+v", c)
		}
	}
	for _, required := range []string{
		"fuchsia-koga-surf-strength",
		"silph-sabrina",
		"cinnabar-blaine",
		"viridian-giovanni",
		"victory-road-indigo",
		"elite-four-champion",
		"fresh-hall-of-fame",
	} {
		if !hasCase(cases, required) {
			t.Errorf("Select(all) omitted runnable %s case", required)
		}
	}
}

func TestLateGameCasesHaveActionsAndPositivePostconditions(t *testing.T) {
	for _, c := range Catalog() {
		switch c.ID {
		case "fuchsia-koga-surf-strength", "silph-sabrina", "cinnabar-blaine", "viridian-giovanni", "victory-road-indigo":
			if !c.Available {
				t.Errorf("%s is still unavailable", c.ID)
			}
			if c.Action == "" {
				t.Errorf("%s has no direct action", c.ID)
			}
			if c.Expect.Kind == "" || c.Expect.Value == "" {
				t.Errorf("%s has no positive semantic postcondition: %+v", c.ID, c.Expect)
			}
		}
	}
}

func TestSelectUnknownCaseAndProfileFail(t *testing.T) {
	if _, err := Select("milestones", "not-a-case"); err == nil {
		t.Fatal("unknown case accepted")
	}
	if _, err := Select("nonsense", ""); err == nil {
		t.Fatal("unknown profile accepted")
	}
}

func caseIDs(cases []Case) []string {
	out := make([]string, len(cases))
	for i, c := range cases {
		out[i] = c.ID
	}
	return out
}

func hasCase(cases []Case, id string) bool {
	for _, c := range cases {
		if c.ID == id {
			return true
		}
	}
	return false
}
