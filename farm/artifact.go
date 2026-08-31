package farm

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"path"
	"unicode"
)

// MaxFinishArtifactBytes is the total artifact payload a FinishReport may
// carry. 24 MiB leaves multipart overhead below Agent Orchestrator's 32 MiB
// default report limit.
const MaxFinishArtifactBytes = 24 << 20

// ValidateFinishArtifacts checks a FinishReport's evidence fields. It does
// not interpret .state contents. Empty artifacts (older runners, scripted
// runs) are valid.
func ValidateFinishArtifacts(r FinishReport) error {
	if r.SeedBurn < 0 {
		return fmt.Errorf("farm: negative seed_burn %d", r.SeedBurn)
	}
	seen := map[string]struct{}{}
	var total int
	for i, a := range r.Artifacts {
		if err := validateArtifact(a); err != nil {
			return fmt.Errorf("farm: artifact %d: %w", i, err)
		}
		if _, ok := seen[a.Name]; ok {
			return fmt.Errorf("farm: duplicate artifact name %q", a.Name)
		}
		seen[a.Name] = struct{}{}
		total += len(a.Data)
		if total > MaxFinishArtifactBytes {
			return fmt.Errorf("farm: artifact payload exceeds %d bytes", MaxFinishArtifactBytes)
		}
	}
	return nil
}

func validateArtifact(a Artifact) error {
	if a.Name == "" {
		return fmt.Errorf("empty name")
	}
	if path.Base(a.Name) != a.Name {
		return fmt.Errorf("name %q is not a bare file name", a.Name)
	}
	if !conservativeArtifactName(a.Name) {
		return fmt.Errorf("name %q is not a conservative ASCII file name", a.Name)
	}
	if len(a.SHA256) != 64 || !isLowerHex(a.SHA256) {
		return fmt.Errorf("sha256 must be 64 lowercase hex characters")
	}
	sum := sha256.Sum256(a.Data)
	want, err := hex.DecodeString(a.SHA256)
	if err != nil {
		return fmt.Errorf("sha256: %w", err)
	}
	if subtle.ConstantTimeCompare(sum[:], want) != 1 {
		return fmt.Errorf("sha256 mismatch")
	}
	return nil
}

func conservativeArtifactName(name string) bool {
	if name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		if r > unicode.MaxASCII {
			return false
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			continue
		}
		return false
	}
	return true
}
