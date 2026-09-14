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

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

const (
	maxPortableBundleDownload = 32 << 20
	maxPortableBundleEntry    = 16 << 20
)

type portableMaterialized struct {
	Manifest          farm.PortableReproManifest
	Dir               string
	StatePath         string
	KnowledgePath     string
	FailureReproPath  string
	WatchdogReproPath string
}

func runPortableBundle(ctx context.Context, source, out string, play bool) error {
	mat, err := materializePortableBundle(ctx, source, out)
	if err != nil {
		return err
	}
	printPortableMaterialized(mat)

	playArgs := "-resume " + shellQuote(mat.StatePath)
	if mat.Manifest.Goal != "" {
		playArgs += " -goal " + shellQuote(mat.Manifest.Goal)
	}
	if mat.Manifest.LLMProfile != "" {
		playArgs += " -llm-profile " + shellQuote(mat.Manifest.LLMProfile)
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

func materializePortableBundle(ctx context.Context, source, out string) (portableMaterialized, error) {
	var mat portableMaterialized
	data, err := loadPortableBundle(ctx, strings.TrimSpace(source))
	if err != nil {
		return mat, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return mat, fmt.Errorf("open bundle zip: %w", err)
	}
	manifestData, err := portableZipFile(zr, "repro.json")
	if err != nil {
		return mat, err
	}
	var manifest farm.PortableReproManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return mat, fmt.Errorf("decode repro.json: %w", err)
	}
	if err := farm.ValidatePortableReproManifest(manifest); err != nil {
		return mat, err
	}
	if manifest.Planner != "" && manifest.Planner != "llm" {
		return mat, fmt.Errorf("portable bundle planner is %q; local checkpoint repro currently supports llm runs", manifest.Planner)
	}
	state, err := portableZipFile(zr, manifest.Checkpoint.Name)
	if err != nil {
		return mat, err
	}
	knowledge, err := portableZipFile(zr, manifest.Knowledge.Name)
	if err != nil {
		return mat, err
	}
	if err := verifyPortableFile(manifest.Checkpoint, state); err != nil {
		return mat, err
	}
	if err := verifyPortableFile(manifest.Knowledge, knowledge); err != nil {
		return mat, err
	}

	dir := strings.TrimSpace(out)
	if dir == "" {
		dir = "repro-" + safeLocal(manifest.RunID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return mat, fmt.Errorf("create %s: %w", dir, err)
	}
	statePath := filepath.Join(dir, manifest.Checkpoint.Name)
	knowledgePath := filepath.Join(dir, manifest.Knowledge.Name)
	if err := os.WriteFile(statePath, state, 0o644); err != nil {
		return mat, fmt.Errorf("write state: %w", err)
	}
	if err := os.WriteFile(knowledgePath, knowledge, 0o644); err != nil {
		return mat, fmt.Errorf("write knowledge: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "repro.json"), manifestData, 0o644); err != nil {
		return mat, fmt.Errorf("write repro.json: %w", err)
	}

	failureReproPath := ""
	failureName := strings.TrimSuffix(manifest.Checkpoint.Name, ".state") + "." + farm.FailureReproArtifactName
	if failureData, ok, err := portableZipFileOptional(zr, failureName); err != nil {
		return mat, err
	} else if ok {
		if _, err := farm.DecodeFailureRepro(failureData); err != nil {
			return mat, fmt.Errorf("decode %s: %w", failureName, err)
		}
		failureReproPath = filepath.Join(dir, failureName)
		if err := os.WriteFile(failureReproPath, failureData, 0o644); err != nil {
			return mat, fmt.Errorf("write %s: %w", failureName, err)
		}
	}

	watchdogReproPath := ""
	if watchdogData, ok, err := portableZipFileOptional(zr, agent.WatchdogReproArtifactName); err != nil {
		return mat, err
	} else if ok {
		var watchdog agent.WatchdogRepro
		if err := json.Unmarshal(watchdogData, &watchdog); err != nil {
			return mat, fmt.Errorf("decode %s: %w", agent.WatchdogReproArtifactName, err)
		}
		if _, err := agent.ReplayWatchdogRepro(watchdog); err != nil {
			return mat, fmt.Errorf("validate %s: %w", agent.WatchdogReproArtifactName, err)
		}
		watchdogReproPath = filepath.Join(dir, agent.WatchdogReproArtifactName)
		if err := os.WriteFile(watchdogReproPath, watchdogData, 0o644); err != nil {
			return mat, fmt.Errorf("write %s: %w", agent.WatchdogReproArtifactName, err)
		}
	}

	return portableMaterialized{
		Manifest:          manifest,
		Dir:               dir,
		StatePath:         statePath,
		KnowledgePath:     knowledgePath,
		FailureReproPath:  failureReproPath,
		WatchdogReproPath: watchdogReproPath,
	}, nil
}

func printPortableMaterialized(mat portableMaterialized) {
	fmt.Printf("materialized portable repro for %s attempt %d checkpoint %s\n", mat.Manifest.RunID, mat.Manifest.Attempt, mat.Manifest.Checkpoint.Name)
	fmt.Printf("  state:     %s\n", mat.StatePath)
	fmt.Printf("  knowledge: %s\n", mat.KnowledgePath)
	if mat.FailureReproPath != "" {
		fmt.Printf("  failure:   %s\n", mat.FailureReproPath)
	}
	if mat.WatchdogReproPath != "" {
		fmt.Printf("  watchdog:  %s\n", mat.WatchdogReproPath)
	}
	if mat.Manifest.ObservedRevision != "" {
		fmt.Printf("  observed:  %s\n", mat.Manifest.ObservedRevision)
	}
	if mat.Manifest.IssueNumber > 0 {
		fmt.Printf("  issue:     #%d\n", mat.Manifest.IssueNumber)
	}
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
	data, ok, err := portableZipFileOptional(zr, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("bundle is missing %q", name)
	}
	return data, nil
}

func portableZipFileOptional(zr *zip.Reader, name string) ([]byte, bool, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		if f.UncompressedSize64 > maxPortableBundleEntry {
			return nil, false, fmt.Errorf("bundle entry %q exceeds %d bytes", name, maxPortableBundleEntry)
		}
		r, err := f.Open()
		if err != nil {
			return nil, false, err
		}
		defer r.Close()
		data, err := io.ReadAll(io.LimitReader(r, maxPortableBundleEntry+1))
		if err != nil {
			return nil, false, err
		}
		if len(data) > maxPortableBundleEntry {
			return nil, false, fmt.Errorf("bundle entry %q exceeds %d bytes", name, maxPortableBundleEntry)
		}
		return data, true, nil
	}
	return nil, false, nil
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
