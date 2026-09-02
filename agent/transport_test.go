package agent

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrTransportIsTypedAndNonRetryable(t *testing.T) {
	err := fmt.Errorf("%w: dial tcp refused", ErrTransport)
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("errors.Is(err, ErrTransport) = false: %v", err)
	}
	if !errors.Is(err, ErrNotFinished) {
		t.Fatalf("transport error does not inherit non-finished classification: %v", err)
	}
	if IsLengthTruncation(err) {
		t.Fatalf("transport error was misread as a length truncation: %v", err)
	}
	if r, retry := classifyRetry(err); retry {
		t.Fatalf("transport error scheduled a model retry %+v", r)
	}
}
