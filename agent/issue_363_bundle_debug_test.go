package agent

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/skill/fixture"
)

func TestIssue363PortableBundleMartClerkSuppressed(t *testing.T) {
	if os.Getenv("POKEMON_RED_ROM") == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}

	const (
		bundleURL = "https://github.com/maestroi/PokePilot/releases/download/farm-repros/repro-issue-363-bea4c755bbf6.zip"
		bundleSHA = "bea4c755bbf64044adefeb344dde0a02e01a5389ec247f4218f8e3dfedddb52a"
	)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(bundleURL)
	if err != nil {
		t.Fatalf("download issue 363 bundle: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download issue 363 bundle: status %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatalf("read issue 363 bundle: %v", err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != bundleSHA {
		t.Fatalf("bundle sha256 = %s, want %s", got, bundleSHA)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open issue 363 bundle: %v", err)
	}
	var stateBytes []byte
	var stateName string
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".state") {
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		stateBytes, err = io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		stateName = f.Name
		break
	}
	if len(stateBytes) == 0 {
		t.Fatal("issue 363 bundle contains no checkpoint state")
	}

	e := fixture.Load(t, "reds_bedroom")
	if err := e.LoadState(stateBytes); err != nil {
		t.Fatalf("LoadState(%s): %v", stateName, err)
	}
	obs := Observe(e, e.ROM())
	clerkX, clerkY, err := rom.MartClerkPosition(e.ROM(), obs.Map)
	if err != nil {
		t.Fatalf("bundle checkpoint map %#04x is not a recognized Mart: %v", obs.Map, err)
	}
	if clerkX != 0 || clerkY != 5 {
		t.Fatalf("bundle checkpoint clerk = (%d,%d), want issue target (0,5)", clerkX, clerkY)
	}

	bad := Objective{Kind: KindTalk, X: 0, Y: 5}
	got := filterRedServiceTalkObjectives(e.ROM(), obs, []Objective{bad})
	if len(got) != 0 {
		t.Fatalf("issue 363 clerk objective still offered on map %#04x: %v", obs.Map, got)
	}
	t.Logf("issue 363 bundle maps to Mart %#04x clerk (0,5); current offer filter suppresses generic TalkAt", obs.Map)
	_ = fmt.Sprintf("%x", obs.Map)
}
