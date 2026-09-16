package main

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestCheckpointHTTPStatusUsesTypedErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "not found", err: fmt.Errorf("context: %w", errStoredCheckpointNotFound), want: http.StatusNotFound},
		{name: "stale", err: fmt.Errorf("context: %w", errCheckpointStale), want: http.StatusConflict},
		{name: "finished", err: fmt.Errorf("context: %w", errCheckpointRunFinished), want: http.StatusConflict},
		{name: "storage unavailable", err: fmt.Errorf("context: %w", errCheckpointStorageUnavailable), want: http.StatusServiceUnavailable},
		{name: "invalid artifact", err: fmt.Errorf("context: %w", errCheckpointArtifactInvalid), want: http.StatusBadRequest},
		{name: "unexpected", err: errors.New("database failed"), want: http.StatusInternalServerError},
		{name: "artifact prose is not policy", err: errors.New("upload checkpoint artifact frame.state: timeout"), want: http.StatusInternalServerError},
		{name: "stale prose is not policy", err: errors.New("stale checkpoint text without typed cause"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkpointHTTPStatus(tt.err); got != tt.want {
				t.Fatalf("checkpointHTTPStatus(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
