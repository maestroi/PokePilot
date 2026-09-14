package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/skill"
)

const (
	verdictObjectiveSucceeded    = "objective_succeeded"
	verdictSameFailureReproduced = "same_failure_reproduced"
	verdictDifferentFailure      = "different_failure"
	verdictContractUnavailable   = "deterministic_contract_unavailable"
	verdictHarnessError          = "harness_error"
)

type portableReproVerdict struct {
	Issue               int64  `json:"issue,omitempty"`
	RunID               string `json:"run_id,omitempty"`
	ObservedRevision    string `json:"observed_revision,omitempty"`
	Classification      string `json:"classification"`
	SourceFingerprint   string `json:"source_fingerprint,omitempty"`
	ObservedFingerprint string `json:"observed_fingerprint,omitempty"`
	Objective           string `json:"objective,omitempty"`
	Outcome             string `json:"outcome,omitempty"`
	Cause               string `json:"cause,omitempty"`
	Diagnostic          string `json:"diagnostic,omitempty"`
}

// verifyPortableBundle replays a structured objective failure directly through
// agent.Execute. No planner is constructed and no model endpoint/token is read:
// the bundle's pre-objective state plus failure-repro contract are the complete
// inputs. Gameplay failures are returned as verdicts, not infrastructure
// errors, so callers can always upload the machine-readable result.
func verifyPortableBundle(mat portableMaterialized, resultPath string) (portableReproVerdict, error) {
	verdict := portableReproVerdict{
		Issue:             mat.Manifest.IssueNumber,
		RunID:             mat.Manifest.RunID,
		ObservedRevision:  mat.Manifest.ObservedRevision,
		SourceFingerprint: mat.Manifest.Fingerprint,
		Objective:         mat.Manifest.Objective,
	}
	write := func() error {
		return writePortableReproVerdict(resultPath, mat.Dir, verdict)
	}

	if mat.FailureReproPath == "" {
		return verifySyntheticFailureBudget(mat, resultPath, verdict)
	}
	failureData, err := os.ReadFile(mat.FailureReproPath)
	if err != nil {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = err.Error()
		_ = write()
		return verdict, fmt.Errorf("read failure repro: %w", err)
	}
	failure, err := farm.DecodeFailureRepro(failureData)
	if err != nil {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = err.Error()
		_ = write()
		return verdict, err
	}
	verdict.SourceFingerprint = failure.Fingerprint
	verdict.Objective = failureObjectiveString(failure.Identity.Objective)

	obj, err := objectiveFromFailure(failure.Identity.Objective)
	if err != nil {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = err.Error()
		_ = write()
		return verdict, err
	}
	romPath := strings.TrimSpace(os.Getenv("POKEMON_RED_ROM"))
	if romPath == "" {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = "POKEMON_RED_ROM is not set"
		_ = write()
		return verdict, fmt.Errorf("POKEMON_RED_ROM is not set")
	}
	stateBytes, err := os.ReadFile(mat.StatePath)
	if err != nil {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = err.Error()
		_ = write()
		return verdict, fmt.Errorf("read checkpoint: %w", err)
	}
	m, err := emu.Open(romPath)
	if err != nil {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = err.Error()
		_ = write()
		return verdict, fmt.Errorf("open ROM: %w", err)
	}
	defer m.Close()
	if err := m.LoadState(stateBytes); err != nil {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = err.Error()
		_ = write()
		return verdict, fmt.Errorf("load checkpoint: %w", err)
	}

	result, execErr := agent.Execute(m, m.ROM(), obj)
	verdict.Outcome = string(result.Outcome)
	verdict.Cause = string(result.Cause)
	if execErr == nil {
		verdict.Classification = verdictObjectiveSucceeded
		verdict.Diagnostic = result.Summary
		return verdict, write()
	}

	observed := failureIdentityFromResult(failure.Identity, result)
	_, observedFingerprint, fingerprintErr := farm.FingerprintFailureIdentity(observed)
	if fingerprintErr != nil {
		verdict.Classification = verdictHarnessError
		verdict.Diagnostic = fingerprintErr.Error()
		_ = write()
		return verdict, fingerprintErr
	}
	verdict.ObservedFingerprint = observedFingerprint
	verdict.Diagnostic = result.Summary
	if observedFingerprint == failure.Fingerprint {
		verdict.Classification = verdictSameFailureReproduced
	} else {
		verdict.Classification = verdictDifferentFailure
	}
	return verdict, write()
}

func writePortableReproVerdict(path, dir string, verdict portableReproVerdict) error {
	path = strings.TrimSpace(path)
	if path == "" {
		path = filepath.Join(dir, "repro-result.json")
	}
	if parent := filepath.Dir(path); parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(verdict, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func objectiveFromFailure(in farm.FailureObjective) (agent.Objective, error) {
	o := agent.Objective{
		Place:    agent.PlaceID(strings.TrimSpace(in.Place)),
		X:        in.X,
		Y:        in.Y,
		Progress: agent.ProgressID(strings.TrimSpace(in.Progress)),
		Level:    in.Level,
		Species:  agent.SpeciesID(strings.TrimSpace(in.Species)),
		Item:     agent.ItemID(strings.TrimSpace(in.Item)),
		Slot:     in.Slot,
		Qty:      in.Qty,
		Flee:     in.Flee,
	}
	switch strings.ToLower(strings.TrimSpace(in.Kind)) {
	case "go_to":
		o.Kind = agent.KindGoTo
	case "talk":
		o.Kind = agent.KindTalk
	case "trainer":
		o.Kind = agent.KindTrainer
	case "starter":
		o.Kind = agent.KindStarter
		switch strings.ToLower(strings.TrimSpace(in.Starter)) {
		case "charmander":
			o.Starter = skill.StarterCharmander
		case "squirtle":
			o.Starter = skill.StarterSquirtle
		case "bulbasaur":
			o.Starter = skill.StarterBulbasaur
		default:
			return agent.Objective{}, fmt.Errorf("failure repro has unknown starter %q", in.Starter)
		}
	case "train":
		o.Kind = agent.KindTrain
	case "heal":
		o.Kind = agent.KindHeal
	case "gym":
		o.Kind = agent.KindGym
	case "catch":
		o.Kind = agent.KindCatch
	case "buy":
		o.Kind = agent.KindBuy
	case "pickup":
		o.Kind = agent.KindPickup
	case "use_item":
		o.Kind = agent.KindUseItem
	case "progress":
		o.Kind = agent.KindProgress
	default:
		return agent.Objective{}, fmt.Errorf("failure repro has unsupported objective kind %q", in.Kind)
	}
	if err := o.Validate(); err != nil {
		return agent.Objective{}, err
	}
	return o, nil
}

func failureIdentityFromResult(source farm.FailureIdentity, result agent.ObjectiveResult) farm.FailureIdentity {
	initial := agent.FailureState{}
	if result.Initial != nil {
		initial = *result.Initial
	}
	return farm.FailureIdentity{
		Version:      farm.FailureIdentityVersion,
		Game:         source.Game,
		Adapter:      source.Adapter,
		Objective:    source.Objective,
		Outcome:      string(result.Outcome),
		Cause:        string(result.Cause),
		CauseContext: append([]string(nil), result.CauseContext...),
		Initial:      farmStateFromAgentFailure(initial),
		Final:        farmStateFromAgentFailure(agent.FailureStateFor(result.Final)),
	}
}

func farmStateFromAgentFailure(s agent.FailureState) farm.FailureState {
	out := farm.FailureState{
		Location:     string(s.Location),
		X:            s.X,
		Y:            s.Y,
		Controllable: s.Controllable,
		InBattle:     s.InBattle,
		Money:        s.Money,
		Badges:       append([]string(nil), s.Badges...),
	}
	for _, mon := range s.Party {
		out.Party = append(out.Party, farm.FailurePartyMember{
			Species: string(mon.Species), Level: mon.Level, HP: mon.HP, MaxHP: mon.MaxHP, Status: mon.Status,
		})
	}
	for _, item := range s.Inventory {
		out.Inventory = append(out.Inventory, farm.FailureInventoryItem{ID: string(item.ID), Quantity: item.Quantity})
	}
	for _, cap := range s.Capabilities {
		out.Capabilities = append(out.Capabilities, farm.FailureCapability{
			ID: string(cap.ID), BadgeOwned: cap.BadgeOwned, HMOwned: cap.HMOwned, Learned: cap.Learned, Usable: cap.Usable,
		})
	}
	for _, fact := range s.Progress {
		out.Progress = append(out.Progress, farm.FailureProgressFact{ID: string(fact.ID), Complete: fact.Complete, Value: fact.Value})
	}
	return out
}

func failureObjectiveString(o farm.FailureObjective) string {
	if obj, err := objectiveFromFailure(o); err == nil {
		return obj.String()
	}
	return strings.TrimSpace(o.Kind)
}
