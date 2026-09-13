package skill

import (
	"errors"
	"testing"
)

func TestMissingFieldRosterPrerequisiteKeepsTypedReplanIdentity(t *testing.T) {
	err := missingFieldRosterPrerequisite(FieldCapability{Name: "CUT", BadgeOwned: false, HMOwned: true})
	if !errors.Is(err, ErrFieldMovePrerequisite) {
		t.Fatalf("missing roster prerequisite %v does not preserve ErrFieldMovePrerequisite", err)
	}
	if !errors.Is(err, ErrFieldRosterPrerequisite) {
		t.Fatalf("missing roster prerequisite %v does not preserve ErrFieldRosterPrerequisite", err)
	}
}

func TestUtilityFieldRecoveryBlockedKeepsTypedReplanIdentity(t *testing.T) {
	err := utilityFieldRecoveryBlocked(FieldCut, ErrFieldRosterNoRecovery,
		"no compatible current-party member, active-box member, or semantically reachable wild species")
	if !errors.Is(err, ErrFieldMovePrerequisite) {
		t.Fatalf("utility recovery error %v does not preserve ErrFieldMovePrerequisite", err)
	}
	if !errors.Is(err, ErrFieldRosterNoRecovery) {
		t.Fatalf("utility recovery error %v does not preserve ErrFieldRosterNoRecovery", err)
	}
}
