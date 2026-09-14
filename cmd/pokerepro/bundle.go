package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/maestroi/pokepilot/farm"
)

const (
	maxPortableBundleDownload = 32 << 20
	maxPortableBundleEntry    = 16 << 20
)

func runPortableBundle(ctx context.Context, source, out string, play bool) error {
	data, err := loadPortableBundle(ctx, strings.TrimSpace(source))
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open bundle zip: %w", err)
	}
	manifestData, err := portableZipFile(zr, "repro.json")
	if err != nil {
		return err
	}
	var manifest farm.PortableReproManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("decode repro.json: %w", err)
	}
	if err := farm.ValidatePortableReproManifest(manifest); err != nil {
		return err
	}
	if manifest.Planner != "" && manifest.Planner != "llm" {
		return fmt.Errorf("portable bundle planner is %q; local checkpoint repro currently supports llm runs", manifest.Planner)
	}
	state, err := portableZipFile(zr, manifest.Checkpoint.Name)
	if err != nil {
		return err
	}
	knowledge, err := portableZipFile(zr, manifest.Knowledge.Name)
	if err != nil {
		return err
	}
	if err := verifyPortableFile(manifest.Checkpoint, state); err != nil {
		return err
	}
	if err := verifyPortableFile(manifest.Knowledge, knowledge); err != nil {
		return err
	}

	dir := strings.TrimSpace(out)
	if dir == "" {
		dir = "repro-" + safeLocal(manifest.RunID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	statePath := filepath.Join(dir, manifest.Checkpoint.Name)
	knowledgePath := filepath.Join(dir, manifest.Knowledge.Name)
	if err := os.WriteFile(statePath, state, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.WriteFile(knowledgePath, knowledge, 0o644); err != nil {
		return fmt.Errorf("write knowledge: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "repro.json"), manifestData, 0o644); err != nil {
		return fmt.Errorf("write repro.json: %w", err)
	}

	fmt.Printf("materialized portable repro for %s attempt %d checkpoint %s\n", manifest.RunID, manifest.Attempt, manifest.Checkpoint.Name)
	fmt.Printf("  state:     %s\n", statePath)
	fmt.Printf("  knowledge: %s\n", knowledgePath)
	if manifest.ObservedRevision != "" {
		fmt.Printf("  observed:  %s\n", manifest.ObservedRevision)
	}
	if manifest.IssueNumber > 0 {
		fmt.Printf("  issue:     #%d\n", manifest.IssueNumber)
	}

	playArgs := "-resume " + shellQuote(statePath)
	if manifest.Goal != "" {
		playArgs += " -goal " + shellQuote(manifest.Goal)
	}
	if manifest.LLMProfile != "" {
		playArgs += " -llm-profile " + shellQuote(manifest.LLMProfile)
	}
	if !play {
		fmt.Printf("\nrun current checkout with:\n  make run-llm ARGS=\"%s\"\n", playArgs)
		return nil
	}

	cmd := exec.Command("make", "run-llm", "ARGS="+playArgs)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("run current checkout: exit %d", exit.ExitCode())
		}
		return fmt.Errorf("run current checkout: %w", err)
	}
	return nil
}

func loadPortableBundle(ctx context.Context, source string) ([]byte, error) {
	if source == "" {
		return nil, fmt.Errorf("bundle source is empty")
	}
	u, err := url.Parse(source)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, err
		}
		client := &http.Client{Timeout: localFetchTimeout}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("download bundle: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("download bundle: status %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxPortableBundleDownload+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxPortableBundleDownload {
			return nil, fmt.Errorf("bundle exceeds %d bytes", maxPortableBundleDownload)
		}
		return data, nil
	}
	info, err := os.Stat(source)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxPortableBundleDownload {
		return nil, fmt.Errorf("bundle exceeds %d bytes", maxPortableBundleDownload)
	}
	return os.ReadFile(source)
}

func portableZipFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		if f.UncompressedSize64 > maxPortableBundleEntry {
			return nil, fmt.Errorf("bundle entry %q exceeds %d bytes", name, maxPortableBundleEntry)
		}
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer r.Close()
		data, err := io.ReadAll(io.LimitReader(r, maxPortableBundleEntry+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxPortableBundleEntry {
			return nil, fmt.Errorf("bundle entry %q exceeds %d bytes", name, maxPortableBundleEntry)
		}
		return data, nil
	}
	return nil, fmt.Errorf("bundle is missing %q", name)
}

func verifyPortableFile(file farm.PortableReproFile, data []byte) error {
	if int64(len(data)) != file.Size {
		return fmt.Errorf("bundle file %s size %d, want %d", file.Name, len(data), file.Size)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, file.SHA256) {
		return fmt.Errorf("bundle file %s sha256 %s, want %s", file.Name, got, file.SHA256)
	}
	return nil
}
