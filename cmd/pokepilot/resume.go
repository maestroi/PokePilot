package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// resolveResume accepts either an exact checkpoint state, a checkpoint
// directory, or a run directory containing checkpoints/. It returns the
// exact state file agent.Run should restore and the directory where new
// checkpoints should continue to be written.
func resolveResume(path string) (statePath, checkpointDir string, err error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("stat %s: %w", path, err)
	}
	if !st.IsDir() {
		if filepath.Ext(path) != ".state" {
			return "", "", fmt.Errorf("%s is not a .state checkpoint", path)
		}
		if _, _, ok := checkpointRoundFrame(filepath.Base(path)); !ok {
			return "", "", fmt.Errorf("%s is not a round checkpoint", path)
		}
		return path, filepath.Dir(path), nil
	}

	checkpointDir = path
	if nested := filepath.Join(path, "checkpoints"); isDir(nested) {
		checkpointDir = nested
	}
	entries, err := os.ReadDir(checkpointDir)
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", checkpointDir, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if _, _, ok := checkpointRoundFrame(entry.Name()); ok {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return "", "", fmt.Errorf("no round checkpoints in %s", checkpointDir)
	}
	sort.Strings(names)
	return filepath.Join(checkpointDir, names[len(names)-1]), checkpointDir, nil
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// checkpointRoundFrame validates the writer's
// round-NNN-frame-NNNNNNNNNN-slug.state naming contract.
func checkpointRoundFrame(name string) (round int, frame uint64, ok bool) {
	const prefix = "round-"
	const frameMarker = "-frame-"
	if !strings.HasSuffix(name, ".state") || !strings.HasPrefix(name, prefix) {
		return 0, 0, false
	}
	i := strings.Index(name, frameMarker)
	if i <= len(prefix) {
		return 0, 0, false
	}
	round, err := strconv.Atoi(name[len(prefix):i])
	if err != nil {
		return 0, 0, false
	}
	rest := name[i+len(frameMarker):]
	j := strings.IndexByte(rest, '-')
	if j <= 0 {
		return 0, 0, false
	}
	frame, err = strconv.ParseUint(rest[:j], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return round, frame, true
}
