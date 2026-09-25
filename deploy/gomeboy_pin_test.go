package deploy_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The replay renderer must be built from the exact GomeBoy release PokePilot
// itself compiles against. A .gbrun embeds a gob-encoded save state, and gob
// rejects a fixed-length array whose length changed: v1.3.0 added the
// SerialExternalClock scheduler event, growing scheduler.State.Events from
// [20]uint64 to [21]uint64.
//
// Pinning gomeboy-stream to the older v1.1.0-era core while the runner moved to
// v1.3.0 made EVERY replay fail to restore its start state:
//
//	gomeboy: ReplayRecording: restore start state: gob: wrong type
//	([20]uint64) for received field State.Events
//
// The renderer then produced no frames and ffmpeg failed with a misleading
// "No filtered frames for output stream" / "A hardware frames reference is
// required" error, so the operator UI's Generate replay button never became
// ready for any run. The two installs must not drift apart again.
func TestGomeboyHelpersMatchModuleVersion(t *testing.T) {
	mod, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	want := gomeboyModuleVersion(t, string(mod))

	dockerfile, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	for _, tool := range []string{"gomeboy-stream", "gomeboy-link-broker"} {
		got := dockerfileToolRef(t, string(dockerfile), tool)
		if got == "" {
			t.Errorf("Dockerfile does not install %s", tool)
			continue
		}
		if got != want {
			t.Errorf("Dockerfile installs %s@%s but go.mod requires gomeboy %s; the replay renderer must match the core that wrote the recording",
				tool, got, want)
		}
	}
}

func gomeboyModuleVersion(t *testing.T, gomod string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^\s*github\.com/maestroi/gomeboy\s+(\S+)`)
	match := re.FindStringSubmatch(gomod)
	if match == nil {
		t.Fatal("go.mod does not require github.com/maestroi/gomeboy")
	}
	return match[1]
}

func dockerfileToolRef(t *testing.T, dockerfile, tool string) string {
	t.Helper()
	re := regexp.MustCompile(`github\.com/maestroi/gomeboy/cmd/` + regexp.QuoteMeta(tool) + `@(\S+)`)
	match := re.FindStringSubmatch(dockerfile)
	if match == nil {
		return ""
	}
	return strings.TrimSpace(match[1])
}
