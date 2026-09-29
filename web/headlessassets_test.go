package webassets

import (
	"bytes"
	"testing"
)

func TestHeadlessAssetsReusePublicBrowserTheme(t *testing.T) {
	manifest, err := PublicThemeManifest()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(manifest, []byte(`"id": "kenney-tiny-town"`)) || !bytes.Contains(manifest, []byte(`"distribution": "public"`)) {
		t.Fatalf("unexpected public manifest: %s", manifest)
	}
	asset, err := PublicThemeAsset("/theme-assets/kenney-tiny-town/tilemap.png#tile=1,0,16")
	if err != nil {
		t.Fatal(err)
	}
	if len(asset) < 8 || !bytes.Equal(asset[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) {
		t.Fatalf("public theme asset is not the expected PNG")
	}
}

func TestHeadlessAssetsRejectLocalOnlyThemeFiles(t *testing.T) {
	if _, err := PublicThemeAsset("/theme-assets/pokegold-gen2/kanto.png"); err == nil {
		t.Fatal("local-only Gold/Silver asset unexpectedly available to public headless renderer")
	}
}
