// Command pokedebug turns one PokePilot run id into a bounded coding-agent
// packet. It intentionally does the expensive/mechanical work outside the
// model: farm evidence compaction, exact artifact retrieval, deterministic
// replay packaging, and local source localization.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/deploy"
	"github.com/maestroi/pokepilot/farm"
)

const debugTimeout = 90 * time.Second

type debugOutput struct {
	Packet        farm.DebugPacket    `json:"packet"`
	PacketPath    string              `json:"packet_path,omitempty"`
	BundlePath    string              `json:"bundle_path,omitempty"`
	Reproduction debugReproduction   `json:"reproduction"`
	SourceMatches []debugSourceMatch  `json:"source_matches,omitempty"`
	Notes         []string            `json:"notes,omitempty"`
}

type debugReproduction struct {
	State         string `json:"state"`
	Classification string `json:"classification,omitempty"`
	ObservedFingerprint string `json:"observed_fingerprint,omitempty"`
	Outcome       string `json:"outcome,omitempty"`
	Cause         string `json:"cause,omitempty"`
	Diagnostic    string `json:"diagnostic,omitempty"`
	ResultPath    string `json:"result_path,omitempty"`
	Output        string `json:"output,omitempty"`
}

type debugSourceMatch struct {
	Term    string `json:"term"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

type artifactContent struct {
	RunID         string `json:"run_id"`
	Name          string `json:"name"`
	MediaType     string `json:"media_type,omitempty"`
	Size          int    `json:"size"`
	ContentBase64 string `json:"content_base64"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "pokedebug:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("pokedebug", flag.ContinueOnError)
	runFlag := fs.String("run", "", "PokePilot run id")
	mode := fs.String("mode", "normal", "packet/source context size: tiny, normal, or deep")
	endpoint := fs.String("endpoint", strings.TrimSpace(os.Getenv("POKEPILOT_MCP_URL")), "PokePilot MCP endpoint")
	token := fs.String("token", strings.TrimSpace(os.Getenv("POKEPILOT_MCP_TOKEN")), "PokePilot MCP bearer token")
	verify := fs.Bool("verify", true, "run the deterministic structured failure replay when supported")
	cache := fs.String("cache", "", "cache/output directory; defaults to the user cache directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	runID := strings.TrimSpace(*runFlag)
	if runID == "" && fs.NArg() > 0 {
		runID = strings.TrimSpace(fs.Arg(0))
	}
	if runID == "" {
		return fmt.Errorf("usage: pokedebug [flags] RUN_ID")
	}
	if strings.TrimSpace(*endpoint) == "" {
		*endpoint = deploy.DefaultMCPEndpoint()
	}
	if strings.TrimSpace(*token) == "" {
		return fmt.Errorf("POKEPILOT_MCP_TOKEN is required (make debug sources ~/.config/pokepilot/env and .env)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), debugTimeout)
	defer cancel()
	raw, err := deploy.CallMCPTool(nil, *endpoint, *token, "pokepilot_prepare_debug", map[string]any{
		"run_id": runID,
		"mode":   strings.TrimSpace(*mode),
	})
	if err != nil {
		return err
	}
	var packet farm.DebugPacket
	if err := json.Unmarshal(raw, &packet); err != nil {
		return fmt.Errorf("decode prepared debug packet: %w", err)
	}
	if packet.Version != farm.DebugPacketVersion {
		return fmt.Errorf("debug packet version %d, want %d", packet.Version, farm.DebugPacketVersion)
	}

	root, rootErr := gitRoot()
	cacheDir, err := debugCacheDir(*cache, runID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create debug cache: %w", err)
	}
	packetPath := filepath.Join(cacheDir, "packet.json")
	if err := writeJSON(packetPath, packet); err != nil {
		return err
	}

	out := debugOutput{
		Packet:     packet,
		PacketPath: packetPath,
		Reproduction: debugReproduction{State: "not_available"},
	}
	if rootErr == nil {
		out.SourceMatches = localizeSource(root, packet.SearchTerms, packet.Mode)
	} else {
		out.Notes = append(out.Notes, "source localization skipped: "+rootErr.Error())
	}

	if packet.Repro.Deterministic && packet.Repro.ContractArtifact != "" {
		bundlePath, bundleErr := materializeDebugBundle(ctx, *endpoint, *token, cacheDir, packet)
		if bundleErr != nil {
			out.Reproduction = debugReproduction{State: "bundle_failed", Diagnostic: bundleErr.Error()}
		} else {
			out.BundlePath = bundlePath
			out.Reproduction.State = "ready"
			if *verify {
				out.Reproduction = verifyDebugBundle(root, bundlePath, cacheDir, packet)
			}
		}
	} else {
		out.Reproduction = debugReproduction{
			State:      "not_available",
			Diagnostic: "no paired structured failure-repro checkpoint is available; expand evidence only if the compact packet is insufficient",
		}
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func debugCacheDir(explicit, runID string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Join(explicit, safeDebugName(runID)), nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "pokepilot", "debug", safeDebugName(runID)), nil
}

func safeDebugName(value string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "run"
	}
	return b.String()
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func gitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	raw, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git checkout")
	}
	root := strings.TrimSpace(string(raw))
	if root == "" {
		return "", fmt.Errorf("git root is empty")
	}
	return root, nil
}

func localizeSource(root string, terms []string, mode string) []debugSourceMatch {
	maxMatches, radius := 6, 7
	switch mode {
	case "tiny":
		maxMatches, radius = 3, 4
	case "deep":
		maxMatches, radius = 12, 14
	}
	var out []debugSourceMatch
	seen := map[string]bool{}
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		cmd := exec.Command("git", "-C", root, "grep", "-n", "-I", "-F", "-e", term, "--", "*.go")
		raw, err := cmd.Output()
		if err != nil && len(raw) == 0 {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			path, lineNo, ok := parseGrepLine(line)
			if !ok || strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/vendor/") {
				continue
			}
			key := path + ":" + strconv.Itoa(lineNo)
			if seen[key] {
				continue
			}
			seen[key] = true
			snippet, err := sourceSnippet(filepath.Join(root, path), lineNo, radius)
			if err != nil {
				continue
			}
			out = append(out, debugSourceMatch{Term: term, Path: path, Line: lineNo, Snippet: snippet})
			if len(out) >= maxMatches {
				return out
			}
		}
	}
	return out
}

func parseGrepLine(line string) (string, int, bool) {
	parts := strings.SplitN(line, ":", 3)
	if len(parts) < 3 {
		return "", 0, false
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 1 {
		return "", 0, false
	}
	return parts[0], n, true
}

func sourceSnippet(path string, line, radius int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	start := line - radius
	if start < 1 {
		start = 1
	}
	end := line + radius
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "%d:%s\n", i, lines[i-1])
		if b.Len() > 3200 {
			break
		}
	}
	return strings.TrimSpace(b.String()), nil
}

func materializeDebugBundle(ctx context.Context, endpoint, token, dir string, packet farm.DebugPacket) (string, error) {
	failureData, err := fetchDebugArtifact(ctx, endpoint, token, packet.RunID, packet.Repro.ContractArtifact)
	if err != nil {
		return "", err
	}
	failure, err := farm.DecodeFailureRepro(failureData)
	if err != nil {
		return "", err
	}
	if failure.Checkpoint.Name != packet.Repro.Checkpoint {
		return "", fmt.Errorf("prepared checkpoint %q disagrees with failure contract %q", packet.Repro.Checkpoint, failure.Checkpoint.Name)
	}
	stateData, err := fetchDebugArtifact(ctx, endpoint, token, packet.RunID, packet.Repro.Checkpoint)
	if err != nil {
		return "", err
	}
	knowledgeData, err := fetchDebugArtifact(ctx, endpoint, token, packet.RunID, packet.Repro.Knowledge)
	if err != nil {
		return "", err
	}

	manifest := farm.PortableReproManifest{
		Version:          farm.PortableReproVersion,
		IssueNumber:      packet.Triage.IssueNumber,
		RunID:            packet.RunID,
		Attempt:          maxInt(1, packet.Finish.Attempt),
		ObservedRevision: firstNonEmpty(packet.Repro.ObservedRevision, packet.Finish.RunnerVersion),
		Fingerprint:      failure.Fingerprint,
		Checkpoint:       portableFile(packet.Repro.Checkpoint, stateData),
		Knowledge:        portableFile(packet.Repro.Knowledge, knowledgeData),
		Planner:          packet.Planner,
		Goal:             packet.Goal,
		Objective:        packet.Failure.Objective,
		Diagnostic:       firstNonEmpty(packet.Repro.Diagnostic, packet.Finish.Detail),
	}
	if err := farm.ValidatePortableReproManifest(manifest); err != nil {
		return "", err
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}

	bundlePath := filepath.Join(dir, "repro.zip")
	file, err := os.Create(bundlePath)
	if err != nil {
		return "", err
	}
	zw := zip.NewWriter(file)
	writeErr := func() error {
		for _, entry := range []struct {
			name string
			data []byte
		}{
			{"repro.json", manifestData},
			{packet.Repro.Checkpoint, stateData},
			{packet.Repro.Knowledge, knowledgeData},
			{packet.Repro.ContractArtifact, failureData},
		} {
			w, err := zw.Create(entry.name)
			if err != nil {
				return err
			}
			if _, err := w.Write(entry.data); err != nil {
				return err
			}
		}
		return nil
	}()
	closeZipErr := zw.Close()
	closeFileErr := file.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeZipErr != nil {
		return "", closeZipErr
	}
	if closeFileErr != nil {
		return "", closeFileErr
	}
	return bundlePath, nil
}

func portableFile(name string, data []byte) farm.PortableReproFile {
	sum := sha256.Sum256(data)
	return farm.PortableReproFile{Name: name, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data))}
}

func fetchDebugArtifact(ctx context.Context, endpoint, token, runID, name string) ([]byte, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("artifact name is empty")
	}
	raw, err := deploy.CallMCPTool(nil, endpoint, token, "pokepilot_get_run_artifact_content", map[string]any{
		"run_id": runID,
		"name":   name,
	})
	if err != nil {
		return nil, err
	}
	var artifact artifactContent
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(artifact.ContentBase64)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	return data, nil
}

func verifyDebugBundle(root, bundlePath, cacheDir string, packet farm.DebugPacket) debugReproduction {
	if root == "" {
		return debugReproduction{State: "skipped", Diagnostic: "not inside a git checkout"}
	}
	game := strings.ToLower(strings.TrimSpace(packet.Game))
	if game != "" && game != "pokemon-red" {
		return debugReproduction{State: "skipped", Diagnostic: "deterministic pokerepro verification currently uses POKEMON_RED_ROM"}
	}
	rom := strings.TrimSpace(os.Getenv("POKEMON_RED_ROM"))
	if rom == "" {
		return debugReproduction{State: "skipped", Diagnostic: "POKEMON_RED_ROM is not set"}
	}
	if _, err := os.Stat(rom); err != nil {
		return debugReproduction{State: "skipped", Diagnostic: "POKEMON_RED_ROM is not readable: " + err.Error()}
	}

	verifyDir := filepath.Join(cacheDir, "verify")
	resultPath := filepath.Join(cacheDir, "repro-result.json")
	_ = os.RemoveAll(verifyDir)
	cmd := exec.Command("go", "run", "./cmd/pokerepro", "-bundle", bundlePath, "-verify", "-out", verifyDir, "-result", resultPath)
	cmd.Dir = root
	cmd.Env = os.Environ()
	raw, runErr := cmd.CombinedOutput()
	result := debugReproduction{
		State:      "ran",
		ResultPath: resultPath,
		Output:     clipDebugOutput(string(raw), 1400),
	}
	var verdict struct {
		Classification      string `json:"classification"`
		ObservedFingerprint string `json:"observed_fingerprint"`
		Outcome             string `json:"outcome"`
		Cause               string `json:"cause"`
		Diagnostic          string `json:"diagnostic"`
	}
	if data, err := os.ReadFile(resultPath); err == nil && json.Unmarshal(data, &verdict) == nil {
		result.Classification = verdict.Classification
		result.ObservedFingerprint = verdict.ObservedFingerprint
		result.Outcome = verdict.Outcome
		result.Cause = verdict.Cause
		result.Diagnostic = verdict.Diagnostic
		switch verdict.Classification {
		case "same_failure_reproduced", "objective_failed":
			result.State = "reproduced"
		case "objective_succeeded":
			result.State = "fixed_or_not_reproduced"
		case "different_failure":
			result.State = "different_failure"
		case "deterministic_contract_unavailable":
			result.State = "unavailable"
		case "harness_error":
			result.State = "harness_error"
		}
	}
	if runErr != nil && result.Classification == "" {
		var exit *exec.ExitError
		if errors.As(runErr, &exit) {
			result.Diagnostic = fmt.Sprintf("pokerepro exited %d", exit.ExitCode())
		} else {
			result.Diagnostic = runErr.Error()
		}
	}
	return result
}

func clipDebugOutput(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return strings.ToValidUTF8(value[:max], "") + "…"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
