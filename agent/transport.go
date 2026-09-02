package agent

import "fmt"

// ErrTransport marks an LLM transport failure: the request never produced a
// usable model reply (dial error, timeout, non-200 response, malformed
// envelope, and similar failures counted in LLMHealth.Transport).
//
// It deliberately unwraps to ErrNotFinished. The existing retry classifier
// already treats a non-length ErrNotFinished as non-retryable, which is the
// correct behavior for infrastructure failure: changing temperature or
// quoting rejection feedback cannot make a busy or unreachable backend
// answer. Callers can still distinguish this class with errors.Is(err,
// ErrTransport).
var ErrTransport = fmt.Errorf("agent: llm planner: transport failure: %w", ErrNotFinished)
