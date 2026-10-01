package compositor

import (
	"encoding/json"
	"fmt"
	"image/color"
	"net/url"
	"strconv"
	"strings"

	webassets "github.com/maestroi/pokepilot/web"
)

const (
	SemanticThemeID      = webassets.PublicThemeID
	semanticThemeVersion = 1
)

func PublicSemanticThemeVersion() string {
	return strconv.Itoa(semanticThemeVersion)
}

type semanticTileStyle struct {
	Fill    string `json:"fill"`
	Detail  string `json:"detail,omitempty"`
	Accent  string `json:"accent,omitempty"`
	Pattern string `json:"pattern,omitempty"`
}

type semanticActorStyle struct {
	Fill   string `json:"fill"`
	Stroke string `json:"stroke"`
}

type semanticThemeManifest struct {
	SchemaVersion         int                           `json:"schemaVersion"`
	ID                    string                        `json:"id"`
	Name                  string                        `json:"name"`
	Version               int                           `json:"version"`
	Distribution          string                        `json:"distribution,omitempty"`
	AllowGameArtFallbacks bool                          `json:"allowGameArtFallbacks,omitempty"`
	TileSize              int                           `json:"tileSize"`
	Tiles                 map[string]semanticTileStyle  `json:"tiles"`
	Objects               map[string]semanticTileStyle  `json:"objects,omitempty"`
	Actors                map[string]semanticActorStyle `json:"actors,omitempty"`
	Animation             struct {
		WaterPeriodMS    int `json:"waterPeriodMs,omitempty"`
		RedrawIntervalMS int `json:"redrawIntervalMs,omitempty"`
	} `json:"animation,omitempty"`
	Effects struct {
		Background string `json:"background,omitempty"`
		Vignette   string `json:"vignette,omitempty"`
		Grid       string `json:"grid,omitempty"`
		Shadow     string `json:"shadow,omitempty"`
	} `json:"effects,omitempty"`
	UI struct {
		Accent string `json:"accent,omitempty"`
		Panel  string `json:"panel,omitempty"`
		Text   string `json:"text,omitempty"`
	} `json:"ui,omitempty"`
	Assets struct {
		Tiles      map[string]string `json:"tiles,omitempty"`
		Characters map[string]string `json:"characters,omitempty"`
		Objects    map[string]string `json:"objects,omitempty"`
		Effects    map[string]string `json:"effects,omitempty"`
		UI         map[string]string `json:"ui,omitempty"`
		Battle     map[string]string `json:"battle,omitempty"`
	} `json:"assets,omitempty"`
	Battle map[string]string `json:"battle,omitempty"`
}

type semanticTheme struct {
	semanticThemeManifest
	actorDefaults map[string]semanticActorStyle
}

func loadPublicSemanticTheme() (semanticTheme, error) {
	data, err := webassets.PublicThemeManifest()
	if err != nil {
		return semanticTheme{}, err
	}
	var manifest semanticThemeManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return semanticTheme{}, fmt.Errorf("decode public semantic theme: %w", err)
	}
	if manifest.SchemaVersion != 1 {
		return semanticTheme{}, fmt.Errorf("semantic theme schema %d is unsupported", manifest.SchemaVersion)
	}
	if manifest.ID != SemanticThemeID {
		return semanticTheme{}, fmt.Errorf("semantic theme id %q, want %q", manifest.ID, SemanticThemeID)
	}
	if manifest.Distribution != "public" {
		return semanticTheme{}, fmt.Errorf("semantic theme %q is not public", manifest.ID)
	}
	if manifest.Version < 1 || manifest.TileSize < 16 || manifest.TileSize > 64 {
		return semanticTheme{}, fmt.Errorf("semantic theme %q has invalid version/tile size", manifest.ID)
	}
	for _, key := range []string{"unknown", "path", "wall"} {
		if strings.TrimSpace(manifest.Tiles[key].Fill) == "" {
			return semanticTheme{}, fmt.Errorf("semantic theme %q is missing tile %q", manifest.ID, key)
		}
	}
	if manifest.Animation.WaterPeriodMS <= 0 {
		manifest.Animation.WaterPeriodMS = 700
	}
	if manifest.Animation.RedrawIntervalMS <= 0 {
		manifest.Animation.RedrawIntervalMS = 100
	}
	if manifest.Effects.Background == "" {
		manifest.Effects.Background = "#142228"
	}
	if manifest.Effects.Vignette == "" {
		manifest.Effects.Vignette = "rgba(0,0,0,.28)"
	}
	if manifest.Effects.Grid == "" {
		manifest.Effects.Grid = "rgba(255,255,255,.045)"
	}
	if manifest.Effects.Shadow == "" {
		manifest.Effects.Shadow = "rgba(0,0,0,.22)"
	}
	if manifest.UI.Accent == "" {
		manifest.UI.Accent = "#67e8f9"
	}
	if manifest.UI.Panel == "" {
		manifest.UI.Panel = "rgba(2,6,23,.72)"
	}
	if manifest.UI.Text == "" {
		manifest.UI.Text = "#cffafe"
	}
	defaults := map[string]semanticActorStyle{
		"player":  {Fill: "#f4f7ff", Stroke: "#e43c4f"},
		"npc":     {Fill: "#f2d071", Stroke: "#18252a"},
		"trainer": {Fill: "#f6a65d", Stroke: "#18252a"},
		"item":    {Fill: "#d9c4ff", Stroke: "#3d315a"},
		"object":  {Fill: "#b8c5ca", Stroke: "#27343a"},
	}
	for key, style := range manifest.Actors {
		if strings.TrimSpace(style.Fill) != "" && strings.TrimSpace(style.Stroke) != "" {
			defaults[key] = style
		}
	}
	return semanticTheme{semanticThemeManifest: manifest, actorDefaults: defaults}, nil
}

func (t semanticTheme) tileStyle(kind string, objectLayer bool) semanticTileStyle {
	if objectLayer {
		if style, ok := t.Objects[kind]; ok {
			return style
		}
	}
	if style, ok := t.Tiles[kind]; ok {
		return style
	}
	return t.Tiles["unknown"]
}

func (t semanticTheme) actorStyle(kind string, player bool) semanticActorStyle {
	if player {
		return t.actorDefaults["player"]
	}
	switch kind {
	case "trainer", "item", "object":
		return t.actorDefaults[kind]
	default:
		return t.actorDefaults["npc"]
	}
}

func (t semanticTheme) battlePaint(key, fallback string) string {
	if value := strings.TrimSpace(t.Battle[key]); value != "" {
		return value
	}
	return fallback
}

func parseSemanticColor(value string, fallback color.RGBA) color.RGBA {
	value = strings.TrimSpace(strings.ToLower(value))
	if strings.HasPrefix(value, "#") {
		hex := strings.TrimPrefix(value, "#")
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		if len(hex) == 6 {
			if n, err := strconv.ParseUint(hex, 16, 32); err == nil {
				return color.RGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}
			}
		}
	}
	parseFunc := func(prefix string) (color.RGBA, bool) {
		if !strings.HasPrefix(value, prefix+"(") || !strings.HasSuffix(value, ")") {
			return color.RGBA{}, false
		}
		inside := strings.TrimSuffix(strings.TrimPrefix(value, prefix+"("), ")")
		parts := strings.Split(inside, ",")
		want := 3
		if prefix == "rgba" {
			want = 4
		}
		if len(parts) != want {
			return color.RGBA{}, false
		}
		channels := [3]uint8{}
		for i := 0; i < 3; i++ {
			n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
			if err != nil || n < 0 || n > 255 {
				return color.RGBA{}, false
			}
			channels[i] = uint8(n)
		}
		alpha := uint8(255)
		if want == 4 {
			a, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
			if err != nil {
				return color.RGBA{}, false
			}
			if a < 0 {
				a = 0
			}
			if a > 1 {
				a = 1
			}
			alpha = uint8(a*255 + 0.5)
		}
		return color.RGBA{R: channels[0], G: channels[1], B: channels[2], A: alpha}, true
	}
	if c, ok := parseFunc("rgba"); ok {
		return c
	}
	if c, ok := parseFunc("rgb"); ok {
		return c
	}
	return fallback
}

type semanticAssetRef struct {
	Reference string
	Path      string
	Column    int
	Row       int
	TileSize  int
	Repeat    int
}

func parseSemanticAssetRef(reference string) (semanticAssetRef, bool) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return semanticAssetRef{}, false
	}
	parsed, err := url.Parse(reference)
	if err != nil || !strings.HasPrefix(parsed.Path, "/theme-assets/") {
		return semanticAssetRef{}, false
	}
	out := semanticAssetRef{
		Reference: reference,
		Path:      parsed.Path,
		Repeat:    1,
	}
	if repeat := strings.TrimSpace(parsed.Query().Get("repeat")); repeat != "" {
		n, err := strconv.Atoi(repeat)
		if err != nil || n < 1 || n > 4 {
			return semanticAssetRef{}, false
		}
		out.Repeat = n
	}
	fragment := parsed.Fragment
	if fragment == "" {
		return out, true
	}
	if !strings.HasPrefix(fragment, "tile=") {
		return semanticAssetRef{}, false
	}
	parts := strings.Split(strings.TrimPrefix(fragment, "tile="), ",")
	if len(parts) != 3 {
		return semanticAssetRef{}, false
	}
	values := [3]int{}
	for i := range parts {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return semanticAssetRef{}, false
		}
		values[i] = n
	}
	if values[2] < 1 || values[2] > 256 {
		return semanticAssetRef{}, false
	}
	out.Column, out.Row, out.TileSize = values[0], values[1], values[2]
	return out, true
}
