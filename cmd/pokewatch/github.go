package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

type githubClient struct {
	http  *http.Client
	base  string
	token string
	repo  string
}

func (g *githubClient) search(q string, perPage int, out any) error {
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/search/issues?per_page=%d&q=%s", g.base, perPage, url.QueryEscape("repo:"+g.repo+" "+q)), nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	res, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("github search: %s", res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// Count returns total_count for a search, or -1 on error.
func (g *githubClient) Count(q string) int {
	var out struct {
		Total int `json:"total_count"`
	}
	if err := g.search(q, 1, &out); err != nil {
		return -1
	}
	return out.Total
}

// TriagePRs lists open "[triage:" pull requests.
func (g *githubClient) TriagePRs() ([]operatorapi.PullRequest, error) {
	var out struct {
		Items []struct {
			Number    int       `json:"number"`
			Title     string    `json:"title"`
			HTMLURL   string    `json:"html_url"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"items"`
	}
	if err := g.search(`is:pr is:open in:title "[triage:"`, 50, &out); err != nil {
		return nil, err
	}
	prs := make([]operatorapi.PullRequest, 0, len(out.Items))
	for _, it := range out.Items {
		if !strings.Contains(it.Title, "[triage:") { // search ignores punctuation
			continue
		}
		prs = append(prs, operatorapi.PullRequest{Number: it.Number, Title: it.Title, URL: it.HTMLURL, CreatedAt: it.CreatedAt.Unix()})
	}
	return prs, nil
}
