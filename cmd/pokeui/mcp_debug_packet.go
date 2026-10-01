package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/farm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpPrepareDebugInput struct {
	RunID string `json:"run_id" jsonschema:"PokePilot run id"`
	Mode  string `json:"mode,omitempty" jsonschema:"packet size: tiny, normal, or deep; defaults to normal"`
	// Attempt and Key scope the evidence to the attempt that failed instead of
	// the run's latest attempt. An endless run keeps failing and recovering, so
	// the latest attempt routinely has no repro bundle while an earlier one
	// does. Attempt wins when both are set; Key resolves through the run's
	// per-attempt problems.
	Attempt int    `json:"attempt,omitempty" jsonschema:"attempt whose failure evidence to prepare; 0 uses the run's latest attempt"`
	Key     string `json:"key,omitempty" jsonschema:"triage failure key identifying the failed attempt; ignored when attempt is set"`
}

var debugIdentifierRE = regexp.MustCompile(`\b[A-Z][A-Za-z0-9_]{2,}\b`)

func (c *mcpControl) prepareDebug(ctx context.Context, _ *mcp.CallToolRequest, in mcpPrepareDebugInput) (*mcp.CallToolResult, farm.DebugPacket, error) {
	id := strings.TrimSpace(in.RunID)
	if id == "" {
		return nil, farm.DebugPacket{}, fmt.Errorf("run_id is required")
	}
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode == "" {
		mode = "normal"
	}
	switch mode {
	case "tiny", "normal", "deep":
	default:
		return nil, farm.DebugPacket{}, fmt.Errorf("mode must be tiny, normal, or deep")
	}

	attempt := c.resolveDebugAttempt(ctx, id, in.Attempt, in.Key)

	debugPath := "/v1/runs/" + url.PathEscape(id) + "/debug"
	if attempt > 0 {
		debugPath += "?attempt=" + strconv.Itoa(attempt)
	}
	var debug map[string]any
	if err := c.requestJSON(ctx, "GET", debugPath, nil, &debug); err != nil {
		return nil, farm.DebugPacket{}, err
	}

	packet := farm.DebugPacket{
		Version: farm.DebugPacketVersion,
		RunID:   id,
		Mode:    mode,
	}
	run, _ := debug["run"].(map[string]any)
	finish, _ := debug["finish"].(map[string]any)
	packet.Status = debugString(run, "status")
	packet.Game = debugString(run, "game")
	packet.Planner = debugString(run, "planner")
	packet.Goal = debugString(run, "goal")
	packet.Attempts = mcpJSONInt(run["attempts"])
	packet.ErrorAttempts = mcpJSONInt(run["error_attempts"])
	packet.Location = farm.DebugLocation{
		Map: uint8(clampByte(mcpJSONInt(run["map"]))),
		X:   uint8(clampByte(mcpJSONInt(run["x"]))),
		Y:   uint8(clampByte(mcpJSONInt(run["y"]))),
	}
	packet.Finish = farm.DebugFinish{
		Attempt:       mcpJSONInt(finish["attempt"]),
		Reason:        debugString(finish, "reason"),
		Detail:        clipDebugText(debugString(finish, "detail"), 320),
		RunnerVersion: debugString(finish, "runner_version"),
	}
	if packet.Finish.Reason == "" {
		packet.Finish.Reason = debugString(run, "reason")
	}
	if packet.Finish.Detail == "" {
		packet.Finish.Detail = clipDebugText(debugString(run, "detail"), 320)
	}

	traceTail := debugStrings(finish["trace_tail"])
	if len(traceTail) > 0 {
		packet.Failure.ErrorChain = clipDebugText(traceTail[len(traceTail)-1], 900)
		packet.Failure.LeafError = debugLeafError(packet.Failure.ErrorChain)
	}
	packet.SearchTerms = debugSearchTerms(packet.Failure.ErrorChain, packet.Failure.LeafError, packet.Finish.Detail)

	// Record which attempt the repro evidence was taken from, so a packet
	// prepared for an older attempt cannot be misread as the latest one.
	packet.Repro.Attempt = attempt

	artifactNames := debugArtifactNames(debug["artifacts"])
	if reproName := latestArtifactWithSuffix(artifactNames, "."+farm.FailureReproArtifactName); reproName != "" {
		packet.Repro.ContractArtifact = reproName
		if _, artifact, err := c.getRunArtifactContent(ctx, nil, mcpArtifactContentInput{RunID: id, Name: reproName, Attempt: attempt}); err == nil {
			if data, decodeErr := base64.StdEncoding.DecodeString(artifact.ContentBase64); decodeErr == nil {
				if bundle, bundleErr := farm.DecodeFailureRepro(data); bundleErr == nil {
					packet.Repro.Checkpoint = bundle.Checkpoint.Name
					packet.Repro.Knowledge = pairedKnowledgeArtifact(artifactNames, bundle.Checkpoint.Name)
					packet.Repro.Fingerprint = bundle.Fingerprint
					packet.Repro.ObservedRevision = bundle.ObservedRevision
					packet.Repro.Diagnostic = clipDebugText(bundle.Diagnostic, 500)
					packet.Repro.Materialize = bundle.SuggestedCommand
					packet.Repro.Deterministic = packet.Repro.Checkpoint != "" && packet.Repro.Knowledge != ""
					packet.Failure.Objective = compactFailureObjective(bundle.Identity.Objective)
					packet.Failure.Cause = bundle.Identity.Cause
					packet.Failure.Outcome = bundle.Identity.Outcome
					packet.SearchTerms = appendUniqueDebugTerms(packet.SearchTerms, debugSearchTerms(bundle.Diagnostic, bundle.Identity.Cause, packet.Failure.Objective)...)
					if len(packet.SearchTerms) > 8 {
						packet.SearchTerms = packet.SearchTerms[:8]
					}
				}
			}
		}
	}
	if packet.Repro.Checkpoint == "" {
		packet.Repro.Checkpoint = latestArtifactWithSuffix(artifactNames, ".state")
		if packet.Repro.Checkpoint != "" {
			packet.Repro.Knowledge = pairedKnowledgeArtifact(artifactNames, packet.Repro.Checkpoint)
		}
	}

	var groups []map[string]any
	if err := c.requestJSON(ctx, "GET", "/v1/triage", nil, &groups); err == nil {
		packet.Triage = compactDebugTriage(groups, run, id)
	}
	if packet.Triage.Fingerprint == "" {
		packet.Triage.Fingerprint = debugStringMap(run, "issue", "fingerprint")
	}
	if packet.Triage.Key == "" {
		packet.Triage.Key = debugString(run, "circuit_key")
	}
	packet.ClassificationHint = debugClassificationHint(packet)
	packet.Evidence = compactDebugEvidence(debug["timeline"], debugEvidenceLimit(mode))
	packet.Next = debugNextSteps(packet)
	return nil, packet, nil
}

// resolveDebugAttempt picks the attempt whose failure evidence the packet
// should describe. An explicit attempt is authoritative; a triage key resolves
// through the run's per-attempt problems to the highest attempt that carried
// it; zero means "the run's latest attempt", which is the historical default.
// A key that matches no attempt also falls back to the latest attempt rather
// than failing the whole evidence handoff.
func (c *mcpControl) resolveDebugAttempt(ctx context.Context, runID string, attempt int, key string) int {
	if attempt > 0 {
		return attempt
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return 0
	}
	var attempts []map[string]any
	if err := c.requestJSON(ctx, "GET", "/v1/runs/"+url.PathEscape(runID)+"/attempts", nil, &attempts); err != nil {
		return 0
	}
	best := 0
	for _, entry := range attempts {
		if !attemptProblemsContainKey(entry["problems"], key) {
			continue
		}
		if candidate := mcpJSONInt(entry["attempt"]); candidate > best {
			best = candidate
		}
	}
	return best
}

func attemptProblemsContainKey(v any, key string) bool {
	problems, _ := v.([]any)
	for _, problem := range problems {
		item, _ := problem.(map[string]any)
		if debugString(item, "triage_key") == key {
			return true
		}
	}
	return false
}

func clampByte(n int) int {
	if n < 0 {
		return 0
	}
	if n > 255 {
		return 255
	}
	return n
}

func debugString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return strings.TrimSpace(v)
}

func debugStringMap(m map[string]any, mapKey, key string) string {
	if m == nil {
		return ""
	}
	nested, _ := m[mapKey].(map[string]any)
	return debugString(nested, key)
}

func debugStrings(v any) []string {
	switch values := v.(type) {
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		return values
	default:
		return nil
	}
}

func debugArtifactNames(v any) []string {
	values, _ := v.([]any)
	out := make([]string, 0, len(values))
	for _, value := range values {
		item, _ := value.(map[string]any)
		name := debugString(item, "name")
		if name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func latestArtifactWithSuffix(names []string, suffix string) string {
	for i := len(names) - 1; i >= 0; i-- {
		if strings.HasSuffix(names[i], suffix) {
			return names[i]
		}
	}
	return ""
}

func pairedKnowledgeArtifact(names []string, checkpoint string) string {
	stem := strings.TrimSuffix(strings.TrimSpace(checkpoint), ".state")
	if stem == "" {
		return ""
	}
	prefix := stem + ".knowledge-v"
	best := ""
	for _, name := range names {
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".json") && name > best {
			best = name
		}
	}
	return best
}

func compactFailureObjective(o farm.FailureObjective) string {
	kind := strings.TrimSpace(o.Kind)
	if kind == "" {
		return ""
	}
	var args []string
	for _, value := range []string{o.Place, o.Progress, o.FieldCapability, o.Species, o.Item, o.Starter} {
		if value = strings.TrimSpace(value); value != "" {
			args = append(args, value)
		}
	}
	if o.Level != 0 {
		args = append(args, "level="+strconv.Itoa(int(o.Level)))
	}
	if o.Qty != 0 {
		args = append(args, "qty="+strconv.Itoa(o.Qty))
	}
	if len(args) == 0 {
		return kind
	}
	return kind + " " + strings.Join(args, " ")
}

func compactDebugTriage(groups []map[string]any, run map[string]any, runID string) farm.DebugTriage {
	key := debugString(run, "circuit_key")
	var selected map[string]any
	for _, group := range groups {
		if key != "" && debugString(group, "key") == key {
			selected = group
			break
		}
		if selected == nil && triageGroupMentionsRun(group, runID) {
			selected = group
		}
	}
	if selected == nil {
		issue, _ := run["issue"].(map[string]any)
		return farm.DebugTriage{
			Key:               key,
			Fingerprint:       debugString(issue, "fingerprint"),
			IssueNumber:       int64(mcpJSONInt(issue["issue_number"])),
			IssueURL:          debugString(issue, "issue_url"),
			Status:            debugString(issue, "status"),
			Resolution:        debugString(issue, "resolution"),
			FixedRevision:     debugString(issue, "fixed_revision"),
			VerificationState: debugString(issue, "verification_state"),
			SolverAttempts:    compactDebugSolverAttempts(issue["solver_attempts"]),
		}
	}
	actionable := triageGroupActionable(selected)
	issue, _ := selected["issue"].(map[string]any)
	return farm.DebugTriage{
		Key:               debugString(selected, "key"),
		Fingerprint:       debugString(selected, "fingerprint"),
		Count:             mcpJSONInt(selected["count"]),
		IssueNumber:       int64(mcpJSONInt(issue["issue_number"])),
		IssueURL:          debugString(issue, "issue_url"),
		Status:            debugString(issue, "status"),
		Resolution:        debugString(issue, "resolution"),
		FixedRevision:     debugString(issue, "fixed_revision"),
		VerificationState: debugString(issue, "verification_state"),
		Actionable:        actionable,
		SolverAttempts:    compactDebugSolverAttempts(issue["solver_attempts"]),
	}
}

func compactDebugSolverAttempts(v any) []farm.DebugSolverAttempt {
	values, _ := v.([]any)
	if len(values) == 0 {
		return nil
	}
	start := len(values) - 3
	if start < 0 {
		start = 0
	}
	out := make([]farm.DebugSolverAttempt, 0, len(values)-start)
	for _, raw := range values[start:] {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		out = append(out, farm.DebugSolverAttempt{
			Backend:  debugString(item, "backend"),
			Model:    debugString(item, "model"),
			State:    debugString(item, "state"),
			RunID:    debugString(item, "run_id"),
			Branch:   debugString(item, "branch"),
			PRNumber: int64(mcpJSONInt(item["pr_number"])),
			PRURL:    debugString(item, "pr_url"),
			Note:     clipDebugText(debugString(item, "note"), 240),
		})
	}
	return out
}

func debugClassificationHint(packet farm.DebugPacket) string {
	status := strings.ToLower(packet.Status)
	reason := strings.ToLower(packet.Finish.Reason)
	detail := strings.ToLower(packet.Finish.Detail)
	chain := strings.ToLower(packet.Failure.ErrorChain)
	if status != "done" && reason == "" {
		return "live_or_pending"
	}
	if strings.Contains(reason, "cancel") || strings.Contains(detail, "cancel") {
		return "cancelled"
	}
	infra := strings.Contains(detail, "no heartbeat") || strings.Contains(detail, "runner drained") || strings.Contains(detail, "runner lost")
	if infra && !strings.Contains(chain, "skill:") && !strings.Contains(chain, "agent:") {
		return "infrastructure_candidate"
	}
	if packet.Repro.Deterministic {
		return "deterministic_defect_candidate"
	}
	if chain != "" && (strings.Contains(chain, "skill:") || strings.Contains(chain, "agent:")) {
		return "defect_candidate"
	}
	return "needs_classification"
}

func debugLeafError(chain string) string {
	chain = strings.TrimSpace(chain)
	if chain == "" {
		return ""
	}
	for _, prefix := range []string{"error: ", "failed: "} {
		chain = strings.TrimPrefix(chain, prefix)
	}
	parts := strings.Split(chain, ": ")
	if len(parts) <= 1 {
		return clipDebugText(chain, 240)
	}
	leaf := strings.TrimSpace(parts[len(parts)-1])
	if leaf == "" {
		leaf = chain
	}
	return clipDebugText(leaf, 240)
}

func debugSearchTerms(values ...string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if phrase := debugStaticPhrase(value); phrase != "" {
			out = appendUniqueDebugTerms(out, phrase)
		}
		for _, id := range debugIdentifierRE.FindAllString(value, -1) {
			switch strings.ToLower(id) {
			case "error", "failed", "failure", "pokemon", "objective", "runner", "attempt", "map":
				continue
			}
			out = appendUniqueDebugTerms(out, id)
			if len(out) >= 8 {
				return out
			}
		}
	}
	return out
}

func debugStaticPhrase(value string) string {
	value = strings.TrimSpace(value)
	for _, marker := range []string{" | ", ": map ", " at map ", " failure-id:", " family-id:"} {
		if i := strings.Index(strings.ToLower(value), strings.ToLower(marker)); i > 0 {
			value = value[:i]
		}
	}
	parts := strings.Fields(value)
	start := 0
	for start < len(parts) && (strings.HasSuffix(parts[start], ":") || strings.EqualFold(parts[start], "skill:") || strings.EqualFold(parts[start], "agent:")) {
		start++
	}
	parts = parts[start:]
	var keep []string
	for _, part := range parts {
		if strings.IndexFunc(part, func(r rune) bool { return r >= '0' && r <= '9' }) >= 0 {
			break
		}
		keep = append(keep, strings.Trim(part, " ,.;:()[]"))
		if len(keep) >= 6 {
			break
		}
	}
	if len(keep) < 3 {
		return ""
	}
	phrase := strings.TrimSpace(strings.Join(keep, " "))
	if len(phrase) > 80 {
		phrase = phrase[:80]
	}
	return phrase
}

func appendUniqueDebugTerms(dst []string, values ...string) []string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		seen := false
		for _, existing := range dst {
			if existing == value {
				seen = true
				break
			}
		}
		if !seen {
			dst = append(dst, value)
		}
	}
	return dst
}

func debugEvidenceLimit(mode string) int {
	switch mode {
	case "tiny":
		return 4
	case "deep":
		return 16
	default:
		return 8
	}
}

func compactDebugEvidence(v any, limit int) []farm.DebugEvidence {
	values, _ := v.([]any)
	if limit <= 0 || len(values) == 0 {
		return nil
	}
	reversed := make([]farm.DebugEvidence, 0, limit)
	for i := len(values) - 1; i >= 0 && len(reversed) < limit; i-- {
		item, _ := values[i].(map[string]any)
		source := strings.ToLower(debugString(item, "source"))
		kind := strings.ToLower(debugString(item, "kind"))
		eventType := strings.ToLower(debugString(item, "type"))
		interesting := source == "recovery" || source == "objective" || source == "skill" ||
			kind == "failure" || kind == "failed" || kind == "retry" || kind == "circuit" ||
			strings.Contains(kind, "objective") || eventType == "progress_final"
		if !interesting {
			continue
		}
		reversed = append(reversed, farm.DebugEvidence{
			Source:  source,
			Kind:    kind,
			Attempt: mcpJSONInt(item["attempt"]),
			Round:   mcpJSONInt(item["round"]),
			Message: clipDebugText(debugString(item, "message"), 240),
			Detail:  clipDebugText(debugString(item, "detail"), 240),
		})
	}
	out := make([]farm.DebugEvidence, len(reversed))
	for i := range reversed {
		out[len(reversed)-1-i] = reversed[i]
	}
	return out
}

func debugNextSteps(packet farm.DebugPacket) []string {
	switch packet.ClassificationHint {
	case "live_or_pending":
		return []string{"inspect live state only if the run is actually stalled; do not create a repair from an unfinished run"}
	case "infrastructure_candidate":
		return []string{"confirm there is no controller error chain before changing gameplay code"}
	case "cancelled":
		return []string{"no repair unless cancellation exposed a separate deterministic controller failure"}
	}
	next := []string{"use the localized source matches before broad repository search"}
	if packet.Repro.Deterministic {
		next = append([]string{"run the deterministic checkpoint replay before editing and again after the patch"}, next...)
	} else {
		next = append([]string{"expand to the full run-debug payload only if the compact evidence cannot establish a reproducer"}, next...)
	}
	next = append(next, "run make test-short after the focused repro is green")
	return next
}

func clipDebugText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return strings.ToValidUTF8(value[:max], "") + "…"
}
