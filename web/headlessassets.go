package webassets

import (
	"embed"
	"fmt"
	"net/url"
	"path"
	"strings"
)

const PublicThemeID = "kenney-tiny-town"

// publicRenderAssets deliberately embeds only the browser assets that are
// distributable on public surfaces. Local-only themes such as pokegold-gen2
// are not present in this filesystem, so the headless media renderer cannot
// accidentally package or access them.
//
//go:embed src/shared/themes/kenney-tiny-town.json public/theme-assets/kenney-tiny-town/tilemap.png public/theme-assets/kenney-tiny-dungeon/tilemap.png
var publicRenderAssets embed.FS

func PublicThemeManifest() ([]byte, error) {
	data, err := publicRenderAssets.ReadFile("src/shared/themes/kenney-tiny-town.json")
	if err != nil {
		return nil, fmt.Errorf("read public render theme: %w", err)
	}
	return data, nil
}

// PublicThemeAsset resolves one browser theme asset reference against the same
// Tiny Town asset files shipped by the public spectator build. Query/fragment
// render hints are intentionally ignored here; callers parse those separately.
func PublicThemeAsset(reference string) ([]byte, error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil {
		return nil, fmt.Errorf("parse public theme asset: %w", err)
	}
	clean := path.Clean("/" + strings.TrimPrefix(parsed.Path, "/"))
	const root = "/theme-assets/"
	if !strings.HasPrefix(clean, root) {
		return nil, fmt.Errorf("public theme asset %q is outside %s", reference, root)
	}
	relative := strings.TrimPrefix(clean, root)
	if !strings.HasPrefix(relative, "kenney-tiny-town/") && !strings.HasPrefix(relative, "kenney-tiny-dungeon/") {
		return nil, fmt.Errorf("public theme asset %q is not distributable", reference)
	}
	data, err := publicRenderAssets.ReadFile("public/theme-assets/" + relative)
	if err != nil {
		return nil, fmt.Errorf("read public theme asset %q: %w", reference, err)
	}
	return data, nil
}
