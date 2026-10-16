// Package publication owns review-first editorial publication decisions.
// It has no dependency on gameplay execution or external upload transports.
package publication

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/media/editplan"
	"github.com/maestroi/pokepilot/media/packaging"
	"github.com/maestroi/pokepilot/media/producer"
)

const (
	SchemaVersion = 1
	PolicyVersion = 1
)

type Format string

const (
	Episode Format = "episode"
	Short   Format = "short"
	Archive Format = "archive"
)

type State string

const (
	Queued     State = "queued"
	Blocked    State = "blocked"
	Rendering  State = "rendering"
	Approved   State = "approved"
	Publishing State = "publishing"
	Published  State = "published"
	Failed     State = "failed"
	Skipped    State = "skipped"
)

type Policy struct {
	Version          int  `json:"version"`
	MinBeats         int  `json:"min_beats"`
	MinAverageScore  int  `json:"min_average_score"`
	MaxSimilarity    int  `json:"max_similarity"`
	RequireThumbnail bool `json:"require_thumbnail"`
}

func DefaultPolicy() Policy {
	return Policy{Version: PolicyVersion, MinBeats: 2, MinAverageScore: 40, MaxSimilarity: 85, RequireThumbnail: true}
}

type Proposal struct {
	RunID          string `json:"run_id"`
	AssessmentHash string `json:"assessment_hash"`
	PackageHash    string `json:"package_hash,omitempty"`
	PlanHash       string `json:"plan_hash,omitempty"`
	Format         Format `json:"format"`
	TitleID        string `json:"title_id,omitempty"`
	ThumbnailID    string `json:"thumbnail_id,omitempty"`
	Privacy        string `json:"privacy"`
	NotBefore      int64  `json:"not_before_unix,omitempty"`
}
type Candidate struct {
	Version         int      `json:"version"`
	ID              string   `json:"id"`
	Proposal        Proposal `json:"proposal"`
	State           State    `json:"state"`
	Approved        bool     `json:"approved"`
	Reason          string   `json:"reason,omitempty"`
	SourceKey       string   `json:"source_key,omitempty"`
	SourceSize      int64    `json:"source_size,omitempty"`
	ThumbnailKey    string   `json:"thumbnail_key,omitempty"`
	ProviderID      string   `json:"provider_id,omitempty"`
	ProviderURL     string   `json:"provider_url,omitempty"`
	ThumbnailStatus string   `json:"thumbnail_status,omitempty"`
	UpdatedAt       int64    `json:"updated_at_unix"`
	Attempts        int      `json:"attempts"`
}

func Hash(value any) string {
	b, _ := json.Marshal(value)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func New(p Proposal) (Candidate, error) {
	if p.RunID == "" || len(p.RunID) > 160 || !hashOK(p.AssessmentHash) || p.NotBefore < 0 {
		return Candidate{}, errors.New("publication: invalid run, assessment or schedule")
	}
	if p.Privacy == "" {
		p.Privacy = "private"
	}
	if p.Privacy != "private" && p.Privacy != "unlisted" && p.Privacy != "public" {
		return Candidate{}, errors.New("publication: invalid privacy")
	}
	if p.Format != Episode && p.Format != Short && p.Format != Archive {
		return Candidate{}, errors.New("publication: invalid format")
	}
	if p.Format != Archive && (!hashOK(p.PackageHash) || !hashOK(p.PlanHash) || p.TitleID == "") {
		return Candidate{}, errors.New("publication: edited media, content package and title selection required")
	}
	if p.Format == Archive && (p.PackageHash != "" || p.PlanHash != "" || p.TitleID != "" || p.ThumbnailID != "") {
		return Candidate{}, errors.New("publication: archived output may not select media")
	}
	return Candidate{Version: SchemaVersion, ID: Hash(p), Proposal: p, State: Queued}, nil
}
func hashOK(h string) bool { return len(h) == 64 && strings.Trim(h, "0123456789abcdef") == "" }
func (c Candidate) Validate() error {
	expected, err := New(c.Proposal)
	if err != nil {
		return err
	}
	if c.Version != SchemaVersion || c.ID != expected.ID {
		return errors.New("publication: candidate identity changed")
	}
	switch c.State {
	case Queued, Blocked, Rendering, Approved, Publishing, Published, Failed, Skipped:
	default:
		return errors.New("publication: invalid lifecycle state")
	}
	if (c.State == Approved || c.State == Publishing || c.State == Published) && !c.Approved {
		return errors.New("publication: publication without approval")
	}
	return nil
}

type Inputs struct {
	Assessment   producer.Assessment
	Package      *packaging.Package
	Plan         *editplan.Plan
	VideoReady   bool
	VideoKey     string
	VideoSize    int64
	ThumbnailKey string
	ThumbnailID  string
}

func AverageScore(s producer.Scores) int {
	return (s.Novelty.Value + s.Drama.Value + s.ChallengeDifficulty.Value + s.FailureAndRecovery.Value) / 4
}
func Evaluate(c Candidate, in Inputs, p Policy) (State, string) {
	if p.Version != PolicyVersion || p.MinBeats < 1 || p.MinAverageScore < 0 || p.MinAverageScore > 100 || p.MaxSimilarity < 0 || p.MaxSimilarity > 100 {
		return Blocked, "unsupported publication quality policy"
	}
	if in.Assessment.RunID != c.Proposal.RunID || in.Assessment.Hash != c.Proposal.AssessmentHash {
		return Blocked, "assessment differs from selected run"
	}
	if in.Assessment.Publication == producer.DoNotPublish || in.Assessment.Publication == producer.ArchiveOnly {
		return Skipped, "producer classification excludes publication"
	}
	if c.Proposal.Format == Archive {
		return Skipped, "operator chose archive-only"
	}
	if c.Proposal.Format == Episode && in.Assessment.Publication != producer.FullEpisode {
		return Blocked, "full episode not authorized by producer"
	}
	if c.Proposal.Format == Short && in.Assessment.Publication != producer.ShortOnly && in.Assessment.Publication != producer.FullEpisode {
		return Blocked, "short not authorized by producer"
	}
	if len(in.Assessment.Beats) < p.MinBeats || AverageScore(in.Assessment.Scores) < p.MinAverageScore {
		return Blocked, "insufficient verified story value"
	}
	if in.Assessment.Scores.RecentSimilarity != nil && in.Assessment.Scores.RecentSimilarity.Value > p.MaxSimilarity {
		return Blocked, "content too similar to recently published episodes"
	}
	if in.Package == nil || in.Package.Hash != c.Proposal.PackageHash || in.Package.AssessmentHash != c.Proposal.AssessmentHash ||
		in.Package.PlanHash != c.Proposal.PlanHash {
		return Blocked, "packaging and edited source do not share an assessment"
	}
	titleFound := false
	if c.Proposal.Format == Short {
		for _, t := range in.Package.Shorts {
			if t.ID == c.Proposal.TitleID && t.Evidence.EventID != "" {
				titleFound = true
				break
			}
		}
	} else {
		for _, t := range in.Package.Titles {
			if t.ID == c.Proposal.TitleID && t.Evidence.EventID != "" {
				titleFound = true
				break
			}
		}
	}
	if !titleFound {
		return Blocked, "title is not backed by recorded evidence"
	}
	if in.Plan == nil || in.Plan.Hash != c.Proposal.PlanHash {
		return Blocked, "missing declarative edit plan"
	}
	if c.Proposal.Format == Episode && in.Plan.Profile != editplan.Episode || c.Proposal.Format == Short && in.Plan.Profile != editplan.Short {
		return Blocked, "video profile does not match publication format"
	}
	if c.Proposal.Format == Episode && p.RequireThumbnail && (c.Proposal.ThumbnailID == "" || in.ThumbnailID != c.Proposal.ThumbnailID || in.ThumbnailKey == "") {
		return Blocked, "select a current rendered thumbnail"
	}
	if c.Proposal.ThumbnailID != "" && in.ThumbnailID != c.Proposal.ThumbnailID {
		return Blocked, "thumbnail selection changed"
	}
	if !in.VideoReady || in.VideoSize <= 0 || in.VideoKey == "" {
		return Rendering, "edited video is not ready"
	}
	return Queued, ""
}
func Approve(c Candidate, in Inputs, p Policy, now time.Time) (Candidate, error) {
	if c.State == Skipped || c.State == Published || c.State == Publishing {
		return c, errors.New("publication: terminal or active candidate cannot be approved")
	}
	state, reason := Evaluate(c, in, p)
	if state != Queued {
		return c, fmt.Errorf("publication: cannot approve: %s", reason)
	}
	c.Approved = true
	c.State = Approved
	c.Reason = ""
	c.SourceKey = in.VideoKey
	c.SourceSize = in.VideoSize
	c.ThumbnailKey = in.ThumbnailKey
	c.UpdatedAt = now.Unix()
	return c, nil
}
func Reject(c Candidate, now time.Time) (Candidate, error) {
	if c.State == Publishing || c.State == Published {
		return c, errors.New("publication: cannot reject upload already started")
	}
	c.State = Skipped
	c.Approved = false
	c.Reason = "rejected by operator"
	c.UpdatedAt = now.Unix()
	return c, nil
}
func CanDispatch(c Candidate, now time.Time) error {
	if !c.Approved || (c.State != Approved && c.State != Failed) {
		return errors.New("publication: explicit approval required")
	}
	if c.Proposal.NotBefore > now.Unix() {
		return errors.New("publication: publication scheduled for later")
	}
	if c.SourceKey == "" || c.SourceSize <= 0 {
		return errors.New("publication: immutable approved source missing")
	}
	return nil
}
