package main

import (
	"path"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
)

func TestStudioPackagesFromKeysReportsPackagesAndThumbnails(t *testing.T) {
	older, newer, orphan := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	prefix := path.Dir(packagingObjectKey("run-1", older)) + "/"
	at := func(sec int64) time.Time { return time.Unix(sec, 0) }
	got := studioPackagesFromKeys(prefix, []artifactstore.ListedObject{
		{Key: prefix + older + ".json", LastModified: at(100)},
		{Key: prefix + newer + ".json", LastModified: at(200)},
		{Key: prefix + "thumbnails/" + older + "/manifest.json"},
		{Key: prefix + "thumbnails/" + older + "/selection.json"},
		{Key: prefix + "thumbnails/" + older + "/images/x.jpg"},
		{Key: prefix + "thumbnails/" + orphan + "/manifest.json"},
		{Key: prefix + "not-a-hash.json", LastModified: at(300)},
	})
	if len(got) != 2 {
		t.Fatalf("packages = %+v, want 2", got)
	}
	if got[0].Hash != newer || got[0].Thumbnails || got[0].ThumbnailSelected {
		t.Fatalf("newest = %+v, want %s without thumbnails", got[0], newer)
	}
	if got[1].Hash != older || !got[1].Thumbnails || !got[1].ThumbnailSelected || got[1].CreatedAtUnixMS != 100_000 {
		t.Fatalf("older = %+v, want thumbnails selected", got[1])
	}
}
