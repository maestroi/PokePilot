package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProgrammingRetryIsProxiedByOperator(t *testing.T) {
	var hits atomic.Int32
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/programming/slot-123/retry" {
			http.Error(w, "unexpected upstream request", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read retry body: %v", err)
		}
		if string(body) != "{}" {
			t.Fatalf("retry body = %q, want {}", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"slot-123","state":"queued"}`))
	}))
	defer wall.Close()

	operator := httptest.NewServer(handlerWithServices(wall.URL, "", ""))
	defer operator.Close()

	resp, err := http.Post(operator.URL+"/v1/programming/slot-123/retry", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("operator retry: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("operator retry = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if hits.Load() != 1 {
		t.Fatalf("operator upstream hits = %d, want 1", hits.Load())
	}
}
