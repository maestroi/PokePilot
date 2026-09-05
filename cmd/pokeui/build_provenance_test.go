package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCurrentBuildProvenance(t *testing.T) {
	oldVersion, oldPR, oldTitle, oldRepo := version, buildPR, buildTitleB64, buildRepo
	t.Cleanup(func() {
		version, buildPR, buildTitleB64, buildRepo = oldVersion, oldPR, oldTitle, oldRepo
	})

	version = "0123456789abcdef"
	buildPR = "61"
	buildTitleB64 = base64.StdEncoding.EncodeToString([]byte("Fix Mt. Moon progression"))
	buildRepo = "maestroi/PokePilot"

	got := currentBuildProvenance()
	if got.Version != version {
		t.Fatalf("Version = %q, want %q", got.Version, version)
	}
	if got.PRNumber != "61" {
		t.Fatalf("PRNumber = %q, want 61", got.PRNumber)
	}
	if got.Title != "Fix Mt. Moon progression" {
		t.Fatalf("Title = %q", got.Title)
	}
	if got.PRURL != "https://github.com/maestroi/PokePilot/pull/61" {
		t.Fatalf("PRURL = %q", got.PRURL)
	}
	if got.CommitURL != "https://github.com/maestroi/PokePilot/commit/0123456789abcdef" {
		t.Fatalf("CommitURL = %q", got.CommitURL)
	}
}

func TestCurrentBuildProvenanceDirectBuildFallback(t *testing.T) {
	oldVersion, oldPR, oldTitle, oldRepo := version, buildPR, buildTitleB64, buildRepo
	t.Cleanup(func() {
		version, buildPR, buildTitleB64, buildRepo = oldVersion, oldPR, oldTitle, oldRepo
	})

	version = "dev"
	buildPR = "0"
	buildTitleB64 = base64.StdEncoding.EncodeToString([]byte("local build"))
	buildRepo = "maestroi/PokePilot"

	got := currentBuildProvenance()
	if got.PRNumber != "" || got.PRURL != "" || got.CommitURL != "" {
		t.Fatalf("dev provenance unexpectedly linked: %+v", got)
	}
	if got.Title != "local build" {
		t.Fatalf("Title = %q, want local build", got.Title)
	}
}

func TestInjectBuildProvenance(t *testing.T) {
	page := []byte(`<html><body><div class="counts"><span class="ver" id="versions"></span></div></body></html>`)
	got := string(injectBuildProvenance(page, buildProvenance{
		Version:   "0123456789abcdef",
		PRNumber:  "61",
		Title:     "Fix Mt. Moon progression",
		PRURL:     "https://github.com/maestroi/PokePilot/pull/61",
		CommitURL: "https://github.com/maestroi/PokePilot/commit/0123456789abcdef",
	}))
	for _, want := range []string{"build-popover", "data-build-label", "Fix Mt. Moon progression", "pull/61", "commit/0123456789abcdef"} {
		if !strings.Contains(got, want) {
			t.Fatalf("injected page missing %q", want)
		}
	}
}

func TestInjectBuildProvenanceLeavesUnrelatedPageAlone(t *testing.T) {
	page := []byte(`<html><body>spectator</body></html>`)
	got := injectBuildProvenance(page, buildProvenance{Version: "0123456"})
	if string(got) != string(page) {
		t.Fatalf("unrelated page changed: %s", got)
	}
}
