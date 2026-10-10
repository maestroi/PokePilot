package farm

// WithPrior returns s with the counters of a run's earlier attempts folded
// in. Every runner attempt starts a fresh planner whose tally begins at
// zero, so a resilient campaign with many attempts would otherwise report
// only its last generation. Live fields (last call, current plan, intent,
// round, goal progress, model identity, record feeds) stay this attempt's;
// counters add and averages are re-weighted by their counts. prior nil
// returns s unchanged.
//
// ponytail: Choices and DecisionSummary stay per-attempt; merge them by key
// if a surface ever needs run-wide objective counts.
func (s *LLMStats) WithPrior(prior *LLMStats) *LLMStats {
	if s == nil || prior == nil {
		return s
	}
	out := *s
	weighted := func(a float64, an int, b float64, bn int) float64 {
		if an+bn == 0 {
			return 0
		}
		return (a*float64(an) + b*float64(bn)) / float64(an+bn)
	}
	out.AvgOffered = weighted(prior.AvgOffered, prior.Calls, s.AvgOffered, s.Calls)
	out.AvgSeconds = weighted(prior.AvgSeconds, prior.Calls, s.AvgSeconds, s.Calls)
	out.SuccessfulAvgSeconds = weighted(prior.SuccessfulAvgSeconds, prior.Calls-prior.Rejected, s.SuccessfulAvgSeconds, s.Calls-s.Rejected)
	out.RejectedAvgSeconds = weighted(prior.RejectedAvgSeconds, prior.Rejected, s.RejectedAvgSeconds, s.Rejected)

	out.Calls += prior.Calls
	out.Rounds += prior.Rounds
	out.Rejected += prior.Rejected
	out.Repeats += prior.Repeats
	out.PromptTokens += prior.PromptTokens
	out.CompletionTokens += prior.CompletionTokens
	out.Transport += prior.Transport
	out.Fallbacks += prior.Fallbacks
	out.Failovers += prior.Failovers

	out.StrategicCalls += prior.StrategicCalls
	out.FastCalls += prior.FastCalls
	out.PlanExecutions += prior.PlanExecutions
	out.LegAutoExecutions += prior.LegAutoExecutions
	out.LegFastExecutions += prior.LegFastExecutions
	out.LegBoundaries += prior.LegBoundaries
	out.LegTailStepsDropped += prior.LegTailStepsDropped
	out.StepsSkipped += prior.StepsSkipped
	out.StrategicSeconds += prior.StrategicSeconds
	out.StrategicRecordsDropped += prior.StrategicRecordsDropped
	if out.StrategicCalls > 0 {
		out.StrategicAvgSeconds = out.StrategicSeconds / float64(out.StrategicCalls)
	}
	if len(prior.ReplanReasons) > 0 {
		reasons := make(map[string]int, len(s.ReplanReasons)+len(prior.ReplanReasons))
		for k, v := range prior.ReplanReasons {
			reasons[k] += v
		}
		for k, v := range s.ReplanReasons {
			reasons[k] += v
		}
		out.ReplanReasons = reasons
	}

	out.DecisionCalls += prior.DecisionCalls
	out.DecisionRejected += prior.DecisionRejected
	out.DecisionFallbacks += prior.DecisionFallbacks
	out.DecisionSeconds += prior.DecisionSeconds
	out.DecisionPromptTokens += prior.DecisionPromptTokens
	out.DecisionCompletionTokens += prior.DecisionCompletionTokens
	out.DecisionInputBytes += prior.DecisionInputBytes
	out.DecisionOutputBytes += prior.DecisionOutputBytes
	out.DecisionRecordsDropped += prior.DecisionRecordsDropped
	out.DecisionAgreements += prior.DecisionAgreements
	out.DecisionDisagreements += prior.DecisionDisagreements
	out.DecisionReferenceAgreements += prior.DecisionReferenceAgreements
	out.DecisionReferenceDisagreements += prior.DecisionReferenceDisagreements
	if out.DecisionCalls > 0 {
		out.DecisionAvgSeconds = out.DecisionSeconds / float64(out.DecisionCalls)
	}
	return &out
}

// Carried is the run-wide tally to keep across an attempt boundary: the
// counters only, without the live feeds that belong to the attempt that
// produced them.
func (s *LLMStats) Carried() *LLMStats {
	if s == nil {
		return nil
	}
	out := *s
	out.StrategicRecords = nil
	out.DecisionRecords = nil
	out.DecisionSummary = nil
	out.Choices = nil
	out.PlanSteps = nil
	out.DecisionProbabilities = nil
	if len(s.ReplanReasons) > 0 {
		out.ReplanReasons = make(map[string]int, len(s.ReplanReasons))
		for k, v := range s.ReplanReasons {
			out.ReplanReasons[k] = v
		}
	}
	return &out
}
