package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/farm"
)

const (
	reproReleaseTag       = "farm-repros"
	maxPortableReproBytes = 24 << 20
)

type portableReproAsset struct {
	Name   string
	URL    string
	SHA256 string
	Size   int64
}

type githubRelease struct {
	ID        int64  `json:"id"`
	TagName   string `json:"tag_name"`
	UploadURL string `json:"upload_url"`
	HTMLURL   string `json:"html_url"`
}

type githubReleaseAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type portableReproBlob struct {
	asset portableReproAsset
	data  []byte
}

// isPortableReproCandidate controls which multipart artifact bytes pokeissues
// retains after hashing. Everything else remains metadata-only so a recording,
// frame, or unrelated diagnostic can never accidentally enter the public ZIP.
func isPortableReproCandidate(name string) bool {
	if !strings.HasPrefix(name, "round-") {
		return false
	}
	return strings.HasSuffix(name, ".state") ||
		(strings.Contains(name, ".knowledge-v") && strings.HasSuffix(name, ".json")) ||
		strings.HasSuffix(name, ".failure-repro.json")
}

func (c *githubClient) attachPortableRepro(ctx context.Context, issue *githubIssue, manifest issueReportManifest, artifacts []artifactMeta) error {
	if issue == nil || issue.Number <= 0 {
		return nil
	}
	if strings.EqualFold(issue.State, "closed") && strings.EqualFold(issue.StateReason, "not_planned") {
		return nil
	}
	blob, ok, err := buildPortableRepro(issue.Number, manifest, artifacts)
	if err != nil || !ok {
		return err
	}
	asset, err := c.publishPortableRepro(ctx, blob)
	if err != nil {
		return err
	}
	return c.ensurePortableReproLink(ctx, issue, manifest, asset)
}

func buildPortableRepro(issueNumber int64, manifest issueReportManifest, artifacts []artifactMeta) (portableReproBlob, bool, error) {
	checkpoint, ok := findReproCheckpoint(artifacts)
	if !ok {
		return portableReproBlob{}, false, nil
	}
	if len(checkpoint.State.Data) == 0 || len(checkpoint.Knowledge.Data) == 0 {
		return portableReproBlob{}, false, fmt.Errorf("portable repro checkpoint bytes were not retained")
	}
	runID := evidenceString(manifest.Evidence, "run_id")
	if runID == "" {
		return portableReproBlob{}, false, nil
	}
	attempt := evidenceInt(manifest.Evidence, "attempt")
	if attempt < 1 {
		attempt = 1
	}

	repro := farm.PortableReproManifest{
		Version:          farm.PortableReproVersion,
		IssueNumber:      issueNumber,
		RunID:            runID,
		Attempt:          attempt,
		ObservedRevision: manifest.ObservedRevision,
		Fingerprint:      manifest.Fingerprint,
		ExternalID:       manifest.ExternalID,
		Checkpoint: farm.PortableReproFile{
			Name: checkpoint.State.Name, SHA256: checkpoint.State.SHA256, Size: checkpoint.State.Size,
		},
		Knowledge: farm.PortableReproFile{
			Name: checkpoint.Knowledge.Name, SHA256: checkpoint.Knowledge.SHA256, Size: checkpoint.Knowledge.Size,
		},
		Planner:    evidenceString(manifest.Evidence, "planner"),
		Goal:       evidenceString(manifest.Evidence, "goal"),
		LLMProfile: evidenceString(manifest.Evidence, "llm_profile"),
		Objective:  evidenceString(manifest.Evidence, "objective"),
		Diagnostic: evidenceString(manifest.Evidence, "error"),
	}
	if err := farm.ValidatePortableReproManifest(repro); err != nil {
		return portableReproBlob{}, false, err
	}
	manifestData, err := json.MarshalIndent(repro, "", "  ")
	if err != nil {
		return portableReproBlob{}, false, err
	}
	manifestData = append(manifestData, '\n')

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, file := range []struct {
		name string
		data []byte
	}{
		{name: "repro.json", data: manifestData},
		{name: checkpoint.State.Name, data: checkpoint.State.Data},
		{name: checkpoint.Knowledge.Name, data: checkpoint.Knowledge.Data},
	} {
		w, err := zw.Create(file.name)
		if err != nil {
			_ = zw.Close()
			return portableReproBlob{}, false, err
		}
		if _, err := w.Write(file.data); err != nil {
			_ = zw.Close()
			return portableReproBlob{}, false, err
		}
	}
	stem := strings.TrimSuffix(checkpoint.State.Name, ".state")
	for _, artifact := range artifacts {
		if artifact.Name != stem+".failure-repro.json" || len(artifact.Data) == 0 {
			continue
		}
		w, err := zw.Create(artifact.Name)
		if err != nil {
			_ = zw.Close()
			return portableReproBlob{}, false, err
		}
		if _, err := w.Write(artifact.Data); err != nil {
			_ = zw.Close()
			return portableReproBlob{}, false, err
		}
		break
	}
	if err := zw.Close(); err != nil {
		return portableReproBlob{}, false, err
	}
	if buf.Len() > maxPortableReproBytes {
		return portableReproBlob{}, false, fmt.Errorf("portable repro exceeds %d bytes", maxPortableReproBytes)
	}
	sum := sha256.Sum256(buf.Bytes())
	sha := hex.EncodeToString(sum[:])
	name := fmt.Sprintf("repro-issue-%d-%s.zip", issueNumber, sha[:12])
	return portableReproBlob{
		asset: portableReproAsset{Name: name, SHA256: sha, Size: int64(buf.Len())},
		data:  append([]byte(nil), buf.Bytes()...),
	}, true, nil
}

func (c *githubClient) publishPortableRepro(ctx context.Context, blob portableReproBlob) (portableReproAsset, error) {
	release, err := c.ensureReproRelease(ctx)
	if err != nil {
		return portableReproAsset{}, err
	}
	assets, err := c.listReleaseAssets(ctx, release.ID)
	if err != nil {
		return portableReproAsset{}, err
	}
	for _, existing := range assets {
		if existing.Name != blob.asset.Name {
			continue
		}
		asset := blob.asset
		asset.URL = existing.BrowserDownloadURL
		asset.Size = existing.Size
		if asset.URL == "" {
			asset.URL = c.releaseAssetURL(blob.asset.Name)
		}
		return asset, nil
	}

	uploaded, err := c.uploadReleaseAsset(ctx, release.UploadURL, blob.asset.Name, blob.data)
	if err != nil {
		return portableReproAsset{}, err
	}
	asset := blob.asset
	asset.URL = uploaded.BrowserDownloadURL
	asset.Size = uploaded.Size
	if asset.URL == "" {
		asset.URL = c.releaseAssetURL(blob.asset.Name)
	}
	return asset, nil
}

func (c *githubClient) ensureReproRelease(ctx context.Context) (githubRelease, error) {
	var release githubRelease
	err := c.doJSON(ctx, http.MethodGet, c.repoPath("releases", "tags", reproReleaseTag), nil, &release)
	if err == nil {
		return release, nil
	}
	var ghErr *githubHTTPError
	if !errors.As(err, &ghErr) || ghErr.Status != http.StatusNotFound {
		return release, err
	}
	payload := map[string]any{
		"tag_name":    reproReleaseTag,
		"name":        "PokePilot farm repro bundles",
		"body":        "Machine-generated, ROM-free checkpoint bundles linked from automated farm issues.",
		"prerelease":  true,
		"make_latest": "false",
	}
	if err := c.doJSON(ctx, http.MethodPost, c.repoPath("releases"), payload, &release); err != nil {
		if errors.As(err, &ghErr) && ghErr.Status == http.StatusUnprocessableEntity {
			if retryErr := c.doJSON(ctx, http.MethodGet, c.repoPath("releases", "tags", reproReleaseTag), nil, &release); retryErr == nil {
				return release, nil
			}
		}
		return githubRelease{}, err
	}
	return release, nil
}

func (c *githubClient) listReleaseAssets(ctx context.Context, releaseID int64) ([]githubReleaseAsset, error) {
	var all []githubReleaseAsset
	for page := 1; ; page++ {
		var assets []githubReleaseAsset
		path := c.repoPath("releases", strconv.FormatInt(releaseID, 10), "assets") + "?per_page=100&page=" + strconv.Itoa(page)
		if err := c.doJSON(ctx, http.MethodGet, path, nil, &assets); err != nil {
			return nil, err
		}
		all = append(all, assets...)
		if len(assets) < 100 {
			return all, nil
		}
	}
}

func (c *githubClient) uploadReleaseAsset(ctx context.Context, uploadURL, name string, data []byte) (githubReleaseAsset, error) {
	var out githubReleaseAsset
	uploadURL = strings.TrimSpace(uploadURL)
	if i := strings.IndexByte(uploadURL, '{'); i >= 0 {
		uploadURL = uploadURL[:i]
	}
	if uploadURL == "" {
		return out, errors.New("github release is missing upload_url")
	}
	u, err := url.Parse(uploadURL)
	if err != nil {
		return out, err
	}
	q := u.Query()
	q.Set("name", name)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(data))
	if err != nil {
		return out, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/zip")
	resp, err := c.http.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGitHubResponse))
	if err != nil {
		return out, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var msg struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &msg)
		return out, &githubHTTPError{Status: resp.StatusCode, Message: msg.Message}
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, fmt.Errorf("decode GitHub release asset response: %w", err)
	}
	return out, nil
}

func (c *githubClient) ensurePortableReproLink(ctx context.Context, issue *githubIssue, manifest issueReportManifest, asset portableReproAsset) error {
	marker := portableReproMarker(manifest.ExternalID)
	section := marker + "\n" + renderPortableReproAsset(asset)
	if strings.Contains(issue.Body, externalIDMarker(manifest.ExternalID)) {
		if strings.Contains(issue.Body, marker) {
			return nil
		}
		body := truncateUTF8(issue.Body, 60<<10) + "\n\n" + section
		if err := c.doJSON(ctx, http.MethodPatch, c.repoPath("issues", strconv.FormatInt(issue.Number, 10)), map[string]string{"body": body}, nil); err != nil {
			return err
		}
		issue.Body = body
		return nil
	}

	path := c.repoPath("issues", strconv.FormatInt(issue.Number, 10), "comments")
	for page := 1; ; page++ {
		var comments []githubComment
		if err := c.doJSON(ctx, http.MethodGet, path+"?per_page=100&page="+strconv.Itoa(page), nil, &comments); err != nil {
			return err
		}
		for _, comment := range comments {
			if strings.Contains(comment.Body, marker) {
				return nil
			}
		}
		if len(comments) < 100 {
			break
		}
	}
	body := marker + "\n### Portable repro bundle\n\nNew farm occurrence: `" + markdownCode(manifest.ExternalID) + "`.\n\n" + renderPortableReproAsset(asset)
	return c.doJSON(ctx, http.MethodPost, path, map[string]string{"body": body}, nil)
}

func renderPortableReproAsset(asset portableReproAsset) string {
	var b strings.Builder
	b.WriteString("### Portable repro bundle\n\n")
	fmt.Fprintf(&b, "- **Download:** [`%s`](%s)\n", markdownCode(asset.Name), asset.URL)
	fmt.Fprintf(&b, "- **ZIP SHA-256:** `%s`\n", asset.SHA256)
	fmt.Fprintf(&b, "- **Bytes:** %d\n", asset.Size)
	b.WriteString("- **Contents:** exact checkpoint state + paired agent knowledge + bounded repro metadata; **no ROM**.\n\n")
	b.WriteString("```bash\n")
	fmt.Fprintf(&b, "go run ./cmd/pokerepro -bundle %s -play\n", shellArg(asset.URL))
	b.WriteString("```\n")
	return b.String()
}

func portableReproMarker(externalID string) string {
	return "<!-- pokepilot-repro-asset:" + htmlCommentSafe(externalID) + " -->"
}

func (c *githubClient) releaseAssetURL(name string) string {
	return strings.TrimRight(c.webBase, "/") + "/" + c.repo + "/releases/download/" + url.PathEscape(reproReleaseTag) + "/" + url.PathEscape(name)
}
