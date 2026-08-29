package agent

import "testing"

// The Goal tests live in the agent package (not agent_test) because the
// byte-identity assertion needs llmSystemPrompt, which is unexported. No
// model is called: systemPrompt is a pure function of the planner fields.

// TestSystemPromptEmptyGoalByteIdentical is the comparability guarantee:
// with no Goal the prompt must be EXACTLY the pre-Goal prompt, so every
// measurement taken before the Goal existed stays comparable to one taken
// after it. A silent rewording here would make prior rows meaningless for
// a reason nobody recorded.
func TestSystemPromptEmptyGoalByteIdentical(t *testing.T) {
	p := &LLMPlanner{}
	if got := p.systemPrompt(); got != llmSystemPrompt {
		t.Fatalf("empty-Goal system prompt is not byte-identical to llmSystemPrompt:\ngot:  %q\nwant: %q", got, llmSystemPrompt)
	}

	// The -inject-fact seam must keep its position relative to the base
	// prompt when a Goal is absent too.
	p.ExtraSystem = "\n\nOne fact about this game: (test fact)."
	if got, want := p.systemPrompt(), llmSystemPrompt+p.ExtraSystem; got != want {
		t.Fatalf("empty-Goal prompt with ExtraSystem changed:\ngot:  %q\nwant: %q", got, want)
	}
}

// TestSystemPromptWithGoal: a non-empty Goal renders as one line above
// everything else and leaves the base prompt (and any ExtraSystem) intact.
func TestSystemPromptWithGoal(t *testing.T) {
	const goal = "Earn the Boulder Badge."
	p := &LLMPlanner{Goal: goal}
	want := "Your goal: " + goal + "\n\n" + llmSystemPrompt
	if got := p.systemPrompt(); got != want {
		t.Fatalf("system prompt with Goal:\ngot:  %q\nwant: %q", got, want)
	}

	// Goal and ExtraSystem are separate seams: the goal is the task
	// statement, the injected fact is the diagnostic. Both may be present,
	// and the goal stays on top.
	p.ExtraSystem = "\n\nOne fact about this game: (test fact)."
	want += p.ExtraSystem
	if got := p.systemPrompt(); got != want {
		t.Fatalf("system prompt with Goal and ExtraSystem:\ngot:  %q\nwant: %q", got, want)
	}
}
