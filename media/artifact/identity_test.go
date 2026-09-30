package artifact

import (
	"strings"
	"testing"
)

func TestNamedProfiles(t *testing.T) {
	for _, name := range []ProfileName{
		ProfileReplayFull,
		ProfileLiveBroadcast,
		ProfileHighlight,
		ProfileClip,
		ProfileShortVertical,
		ProfileDebugRaw,
	} {
		profile, err := NewProfile(name)
		if err != nil {
			t.Fatalf("profile %q: %v", name, err)
		}
		if profile.Name != name || profile.Version != ProfileVersion {
			t.Fatalf("profile %q=%+v", name, profile)
		}
		if profile.Video.Width <= 0 || profile.Video.Height <= 0 || profile.Video.FPSNumerator <= 0 || profile.Video.FPSDenominator <= 0 {
			t.Fatalf("profile %q has incomplete video policy: %+v", name, profile.Video)
		}
	}
	if _, err := NewProfile("unknown"); err == nil {
		t.Fatal("expected unknown profile error")
	}
}

func TestClipAndHighlightProfilesShareMaterialIdentity(t *testing.T) {
	highlight, err := NewProfile(ProfileHighlight)
	if err != nil {
		t.Fatal(err)
	}
	clip, err := NewProfile(ProfileClip)
	if err != nil {
		t.Fatal(err)
	}
	source := []Source{{Attempt: 1, SHA256: strings.Repeat("a", 64)}}
	if got, want := Fingerprint(IdentityInput{Sources: source, Profile: clip}), Fingerprint(IdentityInput{Sources: source, Profile: highlight}); got != want {
		t.Fatalf("clip alias changed material identity: got %s want %s", got, want)
	}
}

func TestFingerprintNormalizesEquivalentInputs(t *testing.T) {
	profile, _ := NewProfile(ProfileReplayFull)
	input := IdentityInput{
		Sources: []Source{
			{Attempt: 2, SHA256: strings.Repeat("B", 64), FallbackID: "ignored"},
			{Attempt: 1, SHA256: strings.Repeat("a", 64)},
		},
		Timelines: []Content{{Attempt: 2, Version: " timeline-v1 ", SHA256: strings.Repeat("C", 64)}},
		Profile:   profile,
	}
	other := input
	other.Sources = []Source{input.Sources[1], input.Sources[0]}
	other.Timelines = []Content{{Attempt: 2, Version: "timeline-v1", SHA256: strings.Repeat("c", 64)}}
	other.Profile.Name = ProfileHighlight
	if got, want := Fingerprint(other), Fingerprint(input); got != want {
		t.Fatalf("equivalent material identity changed: got %s want %s", got, want)
	}
}

func TestFingerprintChangesForMaterialDimensions(t *testing.T) {
	baseProfile, _ := NewProfile(ProfileReplayFull)
	base := IdentityInput{
		Sources:      []Source{{Attempt: 1, SHA256: strings.Repeat("a", 64)}},
		Timelines:    []Content{{Attempt: 1, Version: "timeline-v1", SHA256: strings.Repeat("b", 64)}},
		RenderStates: []Content{{Attempt: 1, Version: "renderstate-v3", SHA256: strings.Repeat("c", 64)}},
		Profile:      baseProfile,
	}
	baseID := Fingerprint(base)

	tests := map[string]func(IdentityInput) IdentityInput{
		"source": func(v IdentityInput) IdentityInput {
			v.Sources = append([]Source(nil), v.Sources...)
			v.Sources[0].SHA256 = strings.Repeat("d", 64)
			return v
		},
		"timeline": func(v IdentityInput) IdentityInput {
			v.Timelines = append([]Content(nil), v.Timelines...)
			v.Timelines[0].SHA256 = strings.Repeat("d", 64)
			return v
		},
		"render state": func(v IdentityInput) IdentityInput {
			v.RenderStates = append([]Content(nil), v.RenderStates...)
			v.RenderStates[0].Version = "renderstate-v4"
			return v
		},
		"renderer": func(v IdentityInput) IdentityInput {
			v.Profile.Presentation.Renderer.Version = "v2"
			return v
		},
		"theme": func(v IdentityInput) IdentityInput {
			v.Profile.Presentation.Theme = Component{ID: "tiny-town", Version: "v2"}
			return v
		},
		"layout": func(v IdentityInput) IdentityInput {
			v.Profile.Presentation.Layout.Version = "v2"
			return v
		},
		"video": func(v IdentityInput) IdentityInput {
			v.Profile.Video.Width++
			return v
		},
		"encoder": func(v IdentityInput) IdentityInput {
			v.Profile.Encoder.Policy = "crf-18"
			return v
		},
		"overlay": func(v IdentityInput) IdentityInput {
			v.Profile.OverlayPolicy = "timeline-v2"
			return v
		},
		"audio": func(v IdentityInput) IdentityInput {
			v.Profile.AudioPolicy = "narration-v1"
			return v
		},
		"segment": func(v IdentityInput) IdentityInput {
			v.Profile.SegmentPolicy.Version = "v2"
			return v
		},
		"edit plan": func(v IdentityInput) IdentityInput {
			v.Profile.EditPlan = &Content{Version: "edit-v1", SHA256: strings.Repeat("e", 64)}
			return v
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Fingerprint(mutate(base)); got == baseID {
				t.Fatalf("%s did not change identity %s", name, got)
			}
		})
	}
}

func TestFingerprintUsesFallbackOnlyWithoutContentHash(t *testing.T) {
	profile, _ := NewProfile(ProfileDebugRaw)
	withHash := IdentityInput{Sources: []Source{{Attempt: 1, SHA256: strings.Repeat("a", 64), FallbackID: "old-key"}}, Profile: profile}
	otherLocation := withHash
	otherLocation.Sources = []Source{{Attempt: 1, SHA256: strings.Repeat("a", 64), FallbackID: "new-key"}}
	if Fingerprint(withHash) != Fingerprint(otherLocation) {
		t.Fatal("object relocation changed identity despite stable content hash")
	}
	withoutHash := IdentityInput{Sources: []Source{{Attempt: 1, FallbackID: "old-key"}}, Profile: profile}
	otherFallback := withoutHash
	otherFallback.Sources = []Source{{Attempt: 1, FallbackID: "new-key"}}
	if Fingerprint(withoutHash) == Fingerprint(otherFallback) {
		t.Fatal("fallback source identity was ignored when no content hash exists")
	}
}
