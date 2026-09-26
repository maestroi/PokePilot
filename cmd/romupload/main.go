// Command romupload publishes every supported cartridge in a directory to the
// farm's S3 ROM store, keyed by the game its bytes are (profiles.Detect), not
// by file name. Runners download a missing game from there on first lease.
//
//	make roms-upload            # upload ./roms
//	go run ./cmd/romupload -n   # dry run: print what would be uploaded
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/profiles"
)

func main() {
	dryRun := flag.Bool("n", false, "print what would be uploaded without uploading")
	flag.Parse()
	dir := "roms"
	if flag.NArg() > 0 {
		dir = flag.Arg(0)
	}
	store, configured, err := artifactstore.S3FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if !configured {
		log.Fatalf("S3 not configured: set %s and friends (.env)", artifactstore.EnvS3Bucket)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		rom, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			log.Fatal(err)
		}
		profile, _, err := profiles.Detect(rom)
		if err != nil {
			fmt.Printf("skip %-24s unsupported by this build\n", entry.Name())
			continue
		}
		key := artifactstore.ROMObjectKey(string(profile.ID()))
		if *dryRun {
			fmt.Printf("would upload %-24s -> s3://%s/%s\n", entry.Name(), store.Bucket(), key)
			continue
		}
		obj, err := store.PutObject(context.Background(), key, "application/octet-stream", rom)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("uploaded %-24s -> s3://%s/%s (%d bytes, sha256 %.12s)\n", entry.Name(), obj.Bucket, obj.Key, obj.Size, obj.SHA256)
	}
}
