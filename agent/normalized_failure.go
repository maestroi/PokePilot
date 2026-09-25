package agent

import gameruntime "github.com/maestroi/pokepilot/game"

func failureClassForOutcome(out Outcome) gameruntime.FailureClass {
	switch out {
	case OutcomeBlocked:
		return gameruntime.FailureClassBlocked
	case OutcomeChoiceRequired:
		return gameruntime.FailureClassChoiceRequired
	case OutcomeStabilizationFailed:
		return gameruntime.FailureClassStabilizationFailed
	case OutcomeOwnershipFailure:
		return gameruntime.FailureClassOwnershipFailure
	case OutcomeControllerUncertain:
		return gameruntime.FailureClassControllerUncertain
	case OutcomePostconditionFailed:
		return gameruntime.FailureClassPostconditionFailed
	case OutcomePostconditionUnavailable:
		return gameruntime.FailureClassPostconditionUnavailable
	default:
		return gameruntime.FailureClassUnknown
	}
}

func outcomeForFailureClass(class gameruntime.FailureClass) Outcome {
	switch class {
	case gameruntime.FailureClassBlocked:
		return OutcomeBlocked
	case gameruntime.FailureClassChoiceRequired:
		return OutcomeChoiceRequired
	case gameruntime.FailureClassStabilizationFailed:
		return OutcomeStabilizationFailed
	case gameruntime.FailureClassOwnershipFailure:
		return OutcomeOwnershipFailure
	case gameruntime.FailureClassControllerUncertain:
		return OutcomeControllerUncertain
	case gameruntime.FailureClassPostconditionFailed:
		return OutcomePostconditionFailed
	case gameruntime.FailureClassPostconditionUnavailable:
		return OutcomePostconditionUnavailable
	default:
		return OutcomeUnknownFailure
	}
}

func failureCauseIs(result ObjectiveResult, cause string) bool {
	if result.Failure != nil && result.Failure.Cause != "" {
		return result.Failure.Cause == cause
	}
	return string(result.Cause) == cause
}

func failureIsBlackout(result ObjectiveResult) bool {
	return failureCauseIs(result, "blacked_out") ||
		failureCauseIs(result, "trainer_blacked_out") ||
		failureCauseIs(result, "catch_blackout") ||
		failureCauseIs(result, failureCauseCombatDefeat)
}

func normalizedFailure(result ObjectiveResult) gameruntime.Failure {
	if result.Failure != nil {
		return *result.Failure
	}
	cause := string(result.Cause)
	if cause == "" && result.Outcome != OutcomeCompleted {
		cause = "outcome:" + string(result.Outcome)
	}
	return gameruntime.Failure{
		Class:       failureClassForOutcome(result.Outcome),
		Cause:       cause,
		Recoverable: actionFor(result.Outcome) == actionReplan,
		Context:     append([]string(nil), result.CauseContext...),
	}
}
