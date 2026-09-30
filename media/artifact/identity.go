package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	ProfileVersion  = 1
	IdentityVersion = 1
)

type ProfileName string

const (
	ProfileReplayFull    ProfileName = "replay-full"
	ProfileLiveBroadcast ProfileName = "live-broadcast"
	ProfileHighlight     ProfileName = "highlight"
	ProfileShortVertical ProfileName = "short-vertical"
	ProfileDebugRaw      ProfileName = "debug-raw"
)

type Component struct {
	ID      string `json:"id,omitempty"`
	Version string `json:"version,omitempty"`
}

type Presentation struct {
	Renderer Component `json:"renderer"`
	Theme    Component `json:"theme,omitempty"`
	Layout   Component `json:"layout"`
}

type Video struct {
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	Aspect         string `json:"aspect"`
	FPSNumerator   int    `json:"fps_numerator"`
	FPSDenominator int    `json:"fps_denominator"`
}

type Encoder struct {
	Codec  string `json:"codec"`
	Policy string `json:"policy"`
}

type Content struct {
	Attempt    int    `json:"attempt,omitempty"`
	Version    string `json:"version,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	FallbackID string `json:"fallback_id,omitempty"`
}

type Profile struct {
	Name          ProfileName  `json:"name"`
	Version       int          `json:"version"`
	Presentation  Presentation `json:"presentation"`
	Video         Video        `json:"video"`
	Encoder       Encoder      `json:"encoder"`
	OverlayPolicy string       `json:"overlay_policy"`
	EventPolicy   string       `json:"event_policy"`
	AudioPolicy   string       `json:"audio_policy"`
	SegmentPolicy Component    `json:"segment_policy"`
	EditPlan      *Content     `json:"edit_plan,omitempty"`
}

type Source struct {
	Attempt    int    `json:"attempt"`
	SHA256     string `json:"sha256,omitempty"`
	FallbackID string `json:"fallback_id,omitempty"`
}

type IdentityInput struct {
	Sources      []Source  `json:"sources"`
	Timelines    []Content `json:"timelines,omitempty"`
	RenderStates []Content `json:"render_states,omitempty"`
	Profile      Profile   `json:"profile"`
}

// NewProfile returns a complete named profile that callers may specialize with
// concrete renderer/theme/encoder versions before computing artifact identity.
func NewProfile(name ProfileName) (Profile, error) {
	profile := Profile{
		Name:          name,
		Version:       ProfileVersion,
		AudioPolicy:   "none",
		SegmentPolicy: Component{ID: "bounded", Version: "v1"},
		Encoder:       Encoder{Codec: "h264", Policy: "default-v1"},
	}
	switch name {
	case ProfileReplayFull:
		profile.Presentation = Presentation{
			Renderer: Component{ID: "classic", Version: "v1"},
			Layout:   Component{ID: "landscape", Version: "v1"},
		}
		profile.Video = Video{Width: 1280, Height: 720, Aspect: "16:9", FPSNumerator: 23891, FPSDenominator: 400}
		profile.OverlayPolicy = "timeline-v1"
		profile.EventPolicy = "events-v1"
	case ProfileLiveBroadcast:
		profile.Presentation = Presentation{
			Renderer: Component{ID: "classic", Version: "v1"},
			Layout:   Component{ID: "landscape", Version: "v1"},
		}
		profile.Video = Video{Width: 1280, Height: 720, Aspect: "16:9", FPSNumerator: 12, FPSDenominator: 1}
		profile.OverlayPolicy = "timeline-v1"
		profile.EventPolicy = "events-v1"
		profile.SegmentPolicy = Component{ID: "live", Version: "v1"}
	case ProfileHighlight:
		profile.Presentation = Presentation{
			Renderer: Component{ID: "classic", Version: "v1"},
			Layout:   Component{ID: "landscape", Version: "v1"},
		}
		profile.Video = Video{Width: 1280, Height: 720, Aspect: "16:9", FPSNumerator: 23891, FPSDenominator: 400}
		profile.OverlayPolicy = "timeline-v1"
		profile.EventPolicy = "highlight-events-v1"
	case ProfileShortVertical:
		profile.Presentation = Presentation{
			Renderer: Component{ID: "semantic", Version: "v1"},
			Layout:   Component{ID: "vertical", Version: "v1"},
		}
		profile.Video = Video{Width: 1080, Height: 1920, Aspect: "9:16", FPSNumerator: 23891, FPSDenominator: 400}
		profile.OverlayPolicy = "timeline-v1"
		profile.EventPolicy = "short-events-v1"
	case ProfileDebugRaw:
		profile.Presentation = Presentation{
			Renderer: Component{ID: "classic", Version: "v1"},
			Layout:   Component{ID: "raw-frame", Version: "v1"},
		}
		profile.Video = Video{Width: 160, Height: 144, Aspect: "10:9", FPSNumerator: 23891, FPSDenominator: 400}
		profile.OverlayPolicy = "none"
		profile.EventPolicy = "none"
	default:
		return Profile{}, fmt.Errorf("unknown media render profile %q", name)
	}
	return profile, nil
}

func NormalizeProfile(profile Profile) Profile {
	if profile.Version == 0 {
		profile.Version = ProfileVersion
	}
	profile.Name = ProfileName(strings.TrimSpace(string(profile.Name)))
	profile.Presentation.Renderer = normalizeComponent(profile.Presentation.Renderer)
	profile.Presentation.Theme = normalizeComponent(profile.Presentation.Theme)
	profile.Presentation.Layout = normalizeComponent(profile.Presentation.Layout)
	profile.Video.Aspect = strings.TrimSpace(profile.Video.Aspect)
	if profile.Video.FPSDenominator == 0 {
		profile.Video.FPSDenominator = 1
	}
	profile.Encoder.Codec = strings.TrimSpace(profile.Encoder.Codec)
	profile.Encoder.Policy = strings.TrimSpace(profile.Encoder.Policy)
	profile.OverlayPolicy = strings.TrimSpace(profile.OverlayPolicy)
	profile.EventPolicy = strings.TrimSpace(profile.EventPolicy)
	profile.AudioPolicy = strings.TrimSpace(profile.AudioPolicy)
	profile.SegmentPolicy = normalizeComponent(profile.SegmentPolicy)
	if profile.EditPlan != nil {
		normalized := normalizeContent(*profile.EditPlan)
		profile.EditPlan = &normalized
	}
	return profile
}

// Fingerprint returns the canonical SHA-256 identity of every input that can
// materially change the media bytes. Profile.Name is intentionally excluded:
// human-friendly aliases are configuration, not cache identity.
func Fingerprint(input IdentityInput) string {
	canonical := canonicalIdentity{
		Version:      IdentityVersion,
		Sources:      normalizeSources(input.Sources),
		Timelines:    normalizeContents(input.Timelines),
		RenderStates: normalizeContents(input.RenderStates),
		Profile:      materialProfile(NormalizeProfile(input.Profile)),
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		panic(fmt.Sprintf("media artifact identity: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func Token(input IdentityInput) string {
	return fmt.Sprintf("v%d-%s", IdentityVersion, Fingerprint(input))
}

type canonicalIdentity struct {
	Version      int             `json:"version"`
	Sources      []Source        `json:"sources"`
	Timelines    []Content       `json:"timelines,omitempty"`
	RenderStates []Content       `json:"render_states,omitempty"`
	Profile      profileMaterial `json:"profile"`
}

type profileMaterial struct {
	Version       int          `json:"version"`
	Presentation  Presentation `json:"presentation"`
	Video         Video        `json:"video"`
	Encoder       Encoder      `json:"encoder"`
	OverlayPolicy string       `json:"overlay_policy"`
	EventPolicy   string       `json:"event_policy"`
	AudioPolicy   string       `json:"audio_policy"`
	SegmentPolicy Component    `json:"segment_policy"`
	EditPlan      *Content     `json:"edit_plan,omitempty"`
}

func materialProfile(profile Profile) profileMaterial {
	return profileMaterial{
		Version:       profile.Version,
		Presentation:  profile.Presentation,
		Video:         profile.Video,
		Encoder:       profile.Encoder,
		OverlayPolicy: profile.OverlayPolicy,
		EventPolicy:   profile.EventPolicy,
		AudioPolicy:   profile.AudioPolicy,
		SegmentPolicy: profile.SegmentPolicy,
		EditPlan:      profile.EditPlan,
	}
}

func normalizeSources(values []Source) []Source {
	out := append([]Source(nil), values...)
	for i := range out {
		out[i].SHA256 = normalizeHash(out[i].SHA256)
		out[i].FallbackID = strings.TrimSpace(out[i].FallbackID)
		if out[i].SHA256 != "" {
			out[i].FallbackID = ""
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Attempt != out[j].Attempt {
			return out[i].Attempt < out[j].Attempt
		}
		if out[i].SHA256 != out[j].SHA256 {
			return out[i].SHA256 < out[j].SHA256
		}
		return out[i].FallbackID < out[j].FallbackID
	})
	return out
}

func normalizeContents(values []Content) []Content {
	out := append([]Content(nil), values...)
	for i := range out {
		out[i] = normalizeContent(out[i])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Attempt != out[j].Attempt {
			return out[i].Attempt < out[j].Attempt
		}
		if out[i].Version != out[j].Version {
			return out[i].Version < out[j].Version
		}
		if out[i].SHA256 != out[j].SHA256 {
			return out[i].SHA256 < out[j].SHA256
		}
		return out[i].FallbackID < out[j].FallbackID
	})
	return out
}

func normalizeContent(value Content) Content {
	value.Version = strings.TrimSpace(value.Version)
	value.SHA256 = normalizeHash(value.SHA256)
	value.FallbackID = strings.TrimSpace(value.FallbackID)
	if value.SHA256 != "" {
		value.FallbackID = ""
	}
	return value
}

func normalizeComponent(value Component) Component {
	value.ID = strings.TrimSpace(value.ID)
	value.Version = strings.TrimSpace(value.Version)
	return value
}

func normalizeHash(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
