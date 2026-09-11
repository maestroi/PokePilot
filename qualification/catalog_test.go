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
		"fuchsia-koga-surf-strength": 33,
		"silph-sabrina":              34,
		"cinnabar-blaine":            35,
		"viridian-giovanni":          36,
		"victory-road-indigo":        37,
		"elite-four-champion":        0,
		"fresh-hall-of-fame":         39,
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
		{profile: "milestones", want: []string{"opening-brock", "mt-moon-cerulean", "misty", "rocket-hideout", "pokemon-tower", "elite-four-champion"}},
		{profile: "full", want: []string{"fresh-hall-of-fame"}},
		{profile: "", want: []string{"opening-brock", "mt-moon-cerulean", "misty", "rocket-hideout", "pokemon-tower", "elite-four-champion"}},
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

func TestSelectAllExcludesFutureUnavailableMilestones(t *testing.T) {
	cases, err := Select("all", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if !c.Available {
			t.Fatalf("Select returned unavailable case %+v", c)
		}
	}
	for _, forbidden := range []string{"fuchsia-koga-surf-strength", "silph-sabrina", "cinnabar-blaine", "viridian-giovanni", "victory-road-indigo"} {
		if hasCase(cases, forbidden) {
			t.Errorf("Select(all) included future case %q", forbidden)
		}
	}
	if !hasCase(cases, "elite-four-champion") {
		t.Error("Select(all) omitted runnable elite-four-champion milestone")
	}
}

func TestSelectPendingCaseFailsWithBlocker(t *testing.T) {
	_, err := Select("milestones", "fuchsia-koga-surf-strength")
	if err == nil || !strings.Contains(err.Error(), "#33") {
		t.Fatalf("err=%v, want blocker #33", err)
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
