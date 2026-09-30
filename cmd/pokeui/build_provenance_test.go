package main

import (
	"encoding/base64"
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

