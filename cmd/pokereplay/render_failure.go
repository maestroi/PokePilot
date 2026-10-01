package main

import (
	"context"
	"errors"
	"strings"

	"github.com/maestroi/pokepilot/farm"
)

func classifyRenderFailure(err error, contextErr error) string {
	if errors.Is(contextErr, context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return farm.MediaRenderFailureTimeout
	}
	if errors.Is(contextErr, context.Canceled) || errors.Is(err, context.Canceled) {
		return farm.MediaRenderFailureCancelled
	}
	message := strings.ToLower(strings.TrimSpace(errString(err)))
	for _, needle := range []string{
		"recording sha256 mismatch",
		"recording: open archive",
		"no mounted replay rom matched",
		"unsupported replay mode",
		"render identity no longer matches",
		"replay segment plan produced no video",
		"decode media timeline",
		"media timeline run_id",
	} {
		if strings.Contains(message, needle) {
			return farm.MediaRenderFailureInvalidRequest
		}
	}
	return farm.MediaRenderFailureInfrastructure
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
