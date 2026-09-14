package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type githubCommitSummary struct {
	SHA string `json:"sha"`
}

type githubCompareResult struct {
	Status string `json:"status"`
}

// fixedRevisionAtClose resolves the default-branch head that existed when a
// completed GitHub issue was closed. Using the closure instant freezes the
// baseline: sampling the current head when a later recurrence arrives would
// incorrectly classify a failure on that current head as stale.
func (c *githubClient) fixedRevisionAtClose(ctx context.Context, issue githubIssue) (string, error) {
	if issue.ClosedAt.IsZero() {
		return "", nil
	}
	path := c.repoPath("commits") + "?until=" + url.QueryEscape(issue.ClosedAt.UTC().Format(time.RFC3339)) + "&per_page=1"
	var commits []githubCommitSummary
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &commits); err != nil {
		return "", err
	}
	if len(commits) == 0 {
		return "", nil
	}
	return strings.TrimSpace(commits[0].SHA), nil
}

// observedRevisionPredatesFix returns true only when observed is a strict
// ancestor of fixed. Equal is deliberately NOT stale: if the build that was
// considered fixed reproduces the failure, the issue must reopen. Diverged or
// otherwise incomparable revisions also fail open as real recurrence candidates.
func (c *githubClient) observedRevisionPredatesFix(ctx context.Context, observed, fixed string) (bool, error) {
	observed = strings.TrimSpace(observed)
	fixed = strings.TrimSpace(fixed)
	if observed == "" || fixed == "" || observed == fixed {
		return false, nil
	}
	var result githubCompareResult
	if err := c.doJSON(ctx, http.MethodGet, c.repoPath("compare", observed+"..."+fixed), nil, &result); err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(result.Status)) {
	case "ahead":
		// base=observed, head=fixed: fixed is ahead, so observed is older.
		return true, nil
	case "identical", "behind", "diverged":
		return false, nil
	default:
		return false, fmt.Errorf("github: unexpected compare status %q for %s...%s", result.Status, observed, fixed)
	}
}

func (c *githubClient) staleRecurrence(ctx context.Context, issue githubIssue, observedRevision string) (fixedRevision string, stale bool, err error) {
	fixedRevision, err = c.fixedRevisionAtClose(ctx, issue)
	if err != nil || fixedRevision == "" || strings.TrimSpace(observedRevision) == "" {
		return fixedRevision, false, err
	}
	stale, err = c.observedRevisionPredatesFix(ctx, observedRevision, fixedRevision)
	return fixedRevision, stale, err
}

func (c *githubClient) ensureStaleOccurrenceComment(ctx context.Context, number int64, manifest issueReportManifest, artifacts []artifactMeta, fixedRevision string) error {
	marker := externalIDMarker(manifest.ExternalID)
	path := c.repoPath("issues", strconv.FormatInt(number, 10), "comments")
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
	payload := map[string]string{"body": renderStaleOccurrenceComment(c.runBase, manifest, artifacts, fixedRevision)}
	return c.doJSON(ctx, http.MethodPost, path, payload, nil)
}

func renderStaleOccurrenceComment(runBase string, manifest issueReportManifest, artifacts []artifactMeta, fixedRevision string) string {
	var b strings.Builder
	b.WriteString(externalIDMarker(manifest.ExternalID))
	b.WriteString("\n### Stale farm recurrence — issue not reopened\n\n")
	fmt.Fprintf(&b, "This occurrence ran revision `%s`, which is older than the fixed-resolution baseline `%s` captured from the default branch at issue closure. It is retained as evidence but does not reopen the issue.\n\n",
		markdownCode(manifest.ObservedRevision), markdownCode(fixedRevision))
	renderMetadata(&b, runBase, manifest)
	b.WriteString("\n")
	b.WriteString(markdownSafeText(truncateUTF8(manifest.Summary, 8<<10)))
	b.WriteString("\n")
	renderReproduction(&b, runBase, manifest, artifacts)
	renderArtifacts(&b, artifacts)
	return truncateUTF8(b.String(), 60<<10)
}
