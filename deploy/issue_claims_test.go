package deploy

import (
	"testing"
	"time"
)

// A leaked assignment (hard-killed attempt, fixer PR closed unmerged, a
// regression reopen) must not park its failure forever; a live one must hold.
func TestIssueClaimsExpireLeakedAssignments(t *testing.T) {
	raw := []byte(`{"data":{"repository":{"issues":{"nodes":[
	 {"number":1,"body":"x\n- **Triage key:** ` + "`fresh`" + `\n","assignees":{"nodes":[{"login":"bot"}]},"timelineItems":{"nodes":[{"createdAt":"2026-10-02T11:00:00Z"}]}},
	 {"number":2,"body":"- **Triage key:** ` + "`leaked`" + `","assignees":{"nodes":[{"login":"bot"},{"login":"me"}]},"timelineItems":{"nodes":[{"createdAt":"2026-10-01T20:24:24Z"}]}},
	 {"number":3,"body":"- **Triage key:** ` + "`withpr`" + `","assignees":{"nodes":[{"login":"bot"}]},"timelineItems":{"nodes":[{"createdAt":"2026-10-01T00:00:00Z"}]}},
	 {"number":4,"body":"- **Triage key:** ` + "`unknown`" + `","assignees":{"nodes":[{"login":"bot"}]},"timelineItems":{"nodes":[]}},
	 {"number":5,"body":"hand-filed, no key","assignees":{"nodes":[{"login":"me"}]},"timelineItems":{"nodes":[]}}
	]}}}}`)
	issues, err := DecodeAssignedIssues(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 4 {
		t.Fatalf("decoded %d keyed issues, want 4: %+v", len(issues), issues)
	}
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	claimed, stale := IssueClaims(issues, []string{"fix(farm): x [triage:withpr]"}, now, 3*time.Hour)
	want := map[string]bool{"fresh": true, "withpr": true, "unknown": true}
	if len(claimed) != len(want) {
		t.Fatalf("claimed = %v, want %v", claimed, want)
	}
	for _, k := range claimed {
		if !want[k] {
			t.Fatalf("claimed = %v, want %v", claimed, want)
		}
	}
	if len(stale) != 1 || stale[0].Number != 2 || stale[0].Key != "leaked" || len(stale[0].Assignees) != 2 {
		t.Fatalf("stale = %+v, want issue #2 with both assignees", stale)
	}
}
