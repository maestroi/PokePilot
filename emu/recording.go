package emu

import (
	"fmt"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

// SessionRecording is the PokePilot-facing handle for one durable GomeBoy
// session recording. The GomeBoy recorder stays behind the emu package
// boundary; callers receive only the encoded .gbrun bytes when the session
// stops.
type SessionRecording struct {
	recorder *gomeboy.SessionRecorder
}

// StartSessionRecording snapshots the emulator's exact current checked state
// and starts recording subsequent joypad transitions. Metadata is opaque to
// the emulator and is copied by GomeBoy when recording starts.
func (m *Emu) StartSessionRecording(metadata map[string]string) (*SessionRecording, error) {
	if m == nil || m.e == nil {
		return nil, fmt.Errorf("emu: start session recording: nil emulator")
	}
	recorder, err := m.e.StartSessionRecording(gomeboy.RecordingOptions{Metadata: metadata})
	if err != nil {
		return nil, err
	}
	return &SessionRecording{recorder: recorder}, nil
}

// Stop finishes the session recording and returns its durable .gbrun archive.
// No filesystem I/O is performed here; farm mode can attach the bytes directly
// to its existing artifact transport.
func (r *SessionRecording) Stop() ([]byte, error) {
	if r == nil || r.recorder == nil {
		return nil, fmt.Errorf("emu: stop session recording: nil recorder")
	}
	recording, err := r.recorder.Stop()
	if err != nil {
		return nil, err
	}
	return recording.MarshalBinary()
}
