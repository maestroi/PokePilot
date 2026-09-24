package farm

// DecisionConfidenceBuckets splits confidence [0,1] into equal-width bins.
const DecisionConfidenceBuckets = 10

// DecisionLatencyEdges are the upper bounds, in seconds, of the latency
// histogram. The final bucket holds everything slower than the last edge.
var DecisionLatencyEdges = [...]float64{0.05, 0.1, 0.2, 0.4, 0.8, 1.6, 3.2, 6.4}

// maxDecisionChoiceKeys bounds the per-kind choice tallies. Objective labels
// are open-ended, so an endless run would otherwise grow them forever.
const maxDecisionChoiceKeys = 24

// DecisionChoiceOther collects choices past maxDecisionChoiceKeys.
const DecisionChoiceOther = "(other)"

// DecisionSummary is a run's whole typed-decision history folded into a
// fixed-size aggregate. It is what a run keeps: individual decisions are only
// streamed through the bounded recent-decision feed and never stored, so the
// summary's size does not depend on how long the run lasts.
type DecisionSummary struct {
	Kinds map[string]*DecisionKindSummary `json:"kinds,omitempty"`
}

// DecisionKindSummary aggregates one decision kind (objective selection,
// failure recovery, and battle turns once those are consulted).
type DecisionKindSummary struct {
	Calls     int `json:"calls"`
	Fallbacks int `json:"fallbacks,omitempty"`
	Errors    int `json:"errors,omitempty"`
	// Shadow counts observed-only answers. Agreements and Disagreements are
	// the shadow answers that could be compared with what executed.
	Shadow        int `json:"shadow,omitempty"`
	Agreements    int `json:"agreements,omitempty"`
	Disagreements int `json:"disagreements,omitempty"`
	// Confidence counts answers per confidence bucket. ConfidenceJudged and
	// ConfidenceAgreed count, per bucket, shadow answers with a verdict and
	// the agreeing ones, so agreement can be read against confidence.
	Confidence       [DecisionConfidenceBuckets]int `json:"confidence"`
	ConfidenceJudged [DecisionConfidenceBuckets]int `json:"confidence_judged"`
	ConfidenceAgreed [DecisionConfidenceBuckets]int `json:"confidence_agreed"`
	// Latency counts calls per DecisionLatencyEdges bucket. P50/P95 are the
	// upper edge of the bucket holding that percentile (the last edge for
	// the overflow bucket).
	Latency          [len(DecisionLatencyEdges) + 1]int `json:"latency"`
	LatencySeconds   float64                            `json:"latency_seconds,omitempty"`
	P50Seconds       float64                            `json:"p50_seconds,omitempty"`
	P95Seconds       float64                            `json:"p95_seconds,omitempty"`
	PromptTokens     int                                `json:"prompt_tokens,omitempty"`
	CompletionTokens int                                `json:"completion_tokens,omitempty"`
	// EngineChoices tallies the engine's picks; ExecutedChoices what the
	// existing policy ran on shadow calls. Keys are choice labels.
	EngineChoices   map[string]int `json:"engine_choices,omitempty"`
	ExecutedChoices map[string]int `json:"executed_choices,omitempty"`
}

// Observe folds one finished decision record into the summary.
func (s *DecisionSummary) Observe(r TypedDecisionRecord) {
	if s.Kinds == nil {
		s.Kinds = map[string]*DecisionKindSummary{}
	}
	k := s.Kinds[r.Kind]
	if k == nil {
		k = &DecisionKindSummary{}
		s.Kinds[r.Kind] = k
	}
	k.Calls++
	if r.Fallback {
		k.Fallbacks++
	}
	if r.Error != "" {
		k.Errors++
	}
	bucket := confidenceBucket(r.Confidence)
	if r.Error == "" {
		k.Confidence[bucket]++
		tally(&k.EngineChoices, choiceKey(r.ChoiceLabel, r.Choice))
	}
	if r.Shadow {
		k.Shadow++
		if r.Executed != "" {
			tally(&k.ExecutedChoices, r.Executed)
		}
		if r.Agreed != nil {
			k.ConfidenceJudged[bucket]++
			if *r.Agreed {
				k.Agreements++
				k.ConfidenceAgreed[bucket]++
			} else {
				k.Disagreements++
			}
		}
	}
	k.Latency[latencyBucket(r.DurationSeconds)]++
	k.LatencySeconds += r.DurationSeconds
	k.P50Seconds = k.latencyPercentile(0.50)
	k.P95Seconds = k.latencyPercentile(0.95)
	k.PromptTokens += r.PromptTokens
	k.CompletionTokens += r.CompletionTokens
}

// Clone deep-copies the summary so heartbeat snapshots never alias the live
// tally.
func (s *DecisionSummary) Clone() *DecisionSummary {
	if s == nil {
		return nil
	}
	out := &DecisionSummary{Kinds: make(map[string]*DecisionKindSummary, len(s.Kinds))}
	for kind, k := range s.Kinds {
		c := *k
		c.EngineChoices = cloneCounts(k.EngineChoices)
		c.ExecutedChoices = cloneCounts(k.ExecutedChoices)
		out.Kinds[kind] = &c
	}
	return out
}

func (k *DecisionKindSummary) latencyPercentile(p float64) float64 {
	total := 0
	for _, n := range k.Latency {
		total += n
	}
	if total == 0 {
		return 0
	}
	rank := int(p*float64(total) + 0.5)
	if rank < 1 {
		rank = 1
	}
	seen := 0
	for i, n := range k.Latency {
		seen += n
		if seen >= rank {
			if i < len(DecisionLatencyEdges) {
				return DecisionLatencyEdges[i]
			}
			break
		}
	}
	return DecisionLatencyEdges[len(DecisionLatencyEdges)-1]
}

func confidenceBucket(c float64) int {
	b := int(c * DecisionConfidenceBuckets)
	if b < 0 {
		return 0
	}
	if b >= DecisionConfidenceBuckets {
		return DecisionConfidenceBuckets - 1
	}
	return b
}

func latencyBucket(seconds float64) int {
	for i, edge := range DecisionLatencyEdges {
		if seconds <= edge {
			return i
		}
	}
	return len(DecisionLatencyEdges)
}

func choiceKey(label, id string) string {
	if label != "" {
		return label
	}
	return id
}

func tally(m *map[string]int, key string) {
	if key == "" {
		return
	}
	if *m == nil {
		*m = map[string]int{}
	}
	if _, ok := (*m)[key]; !ok && len(*m) >= maxDecisionChoiceKeys {
		key = DecisionChoiceOther
	}
	(*m)[key]++
}

func cloneCounts(in map[string]int) map[string]int {
	if in == nil {
		return nil
	}
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
