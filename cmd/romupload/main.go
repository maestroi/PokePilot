// Command romupload publishes every recognised cartridge in a ROM directory
// to the farm's ROM store (roms/<game id>) unless it is already there, so a
// runner on any Swarm node can fetch a game it was not given on disk.
// Identity comes from the bytes (profiles.DetectCartridge), never the name.
//
//	make roms-upload            # uploads ./roms
//	go run ./cmd/romupload -dir /path/to/roms
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/profiles"
)

// romObjectPrefix must match cmd/pokepilot's ROM library fetch key.
const romObjectPrefix = "roms/"

func main() {
	dir := flag.String("dir", "roms", "directory of ROM files to upload")
	flag.Parse()
	store, configured, err := artifactstore.S3FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if !configured {
		log.Fatalf("S3 not configured: set %s (and endpoint/credentials) in .env", artifactstore.EnvS3Bucket)
	}
	if err := upload(context.Background(), store, *dir, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func upload(ctx context.Context, store *artifactstore.S3, dir string, out io.Writer) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		rom, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		profile, _, err := profiles.DetectCartridge(rom)
		if err != nil {
			fmt.Fprintf(out, "skip    %s: %v\n", entry.Name(), err)
			continue
		}
		key := romObjectPrefix + string(profile.ID())
		if _, err := store.HeadObject(ctx, key); err == nil {
			fmt.Fprintf(out, "exists  %s -> %s\n", entry.Name(), key)
			continue
		} else if !artifactstore.IsNotFound(err) {
			return err
		}
		if _, err := store.PutObject(ctx, key, "application/octet-stream", rom); err != nil {
			return err
		}
		fmt.Fprintf(out, "upload  %s -> %s\n", entry.Name(), key)
	}
	return nil
}
