package main

import (
	"encoding/base64"
	"strconv"
	"strings"
)

// Build provenance is injected by the farm image build. The title is base64
// encoded before it reaches the linker so arbitrary PR punctuation cannot turn
// into shell syntax in the Docker build.
var (
	buildPR       = "0"
	buildTitleB64 = ""
	buildRepo     = "maestroi/PokePilot"
)

type buildProvenance struct {
	Version   string `json:"version"`
	PRNumber  string `json:"pr_number,omitempty"`
	Title     string `json:"title,omitempty"`
	PRURL     string `json:"pr_url,omitempty"`
	CommitURL string `json:"commit_url,omitempty"`
}

func currentBuildProvenance() buildProvenance {
	p := buildProvenance{Version: strings.TrimSpace(version)}
	if raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(buildTitleB64)); err == nil {
		p.Title = strings.TrimSpace(string(raw))
	}

	if n, err := strconv.Atoi(strings.TrimSpace(buildPR)); err == nil && n > 0 {
		p.PRNumber = strconv.Itoa(n)
	}

	repo := strings.Trim(strings.TrimSpace(buildRepo), "/")
	if !validRepositorySlug(repo) {
		return p
	}
	base := "https://github.com/" + repo
	if p.PRNumber != "" {
		p.PRURL = base + "/pull/" + p.PRNumber
	}
	if validGitSHA(p.Version) {
		p.CommitURL = base + "/commit/" + p.Version
	}
	return p
}

func validRepositorySlug(repo string) bool {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	for _, part := range parts {
		for _, r := range part {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("-_.", r) {
				continue
			}
			return false
		}
	}
	return true
}

func validGitSHA(sha string) bool {
	if len(sha) < 7 || len(sha) > 64 {
		return false
	}
	for _, r := range sha {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}
