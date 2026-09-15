package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWorkerControlForceEndAcknowledgesBeforeExit(t *testing.T) {
	exited := make(chan int, 1)
	server := httptest.NewServer(newWorkerControlHandler(func(code int) { exited <- code }))
	defer server.Close()

	resp, err := http.Post(server.URL+"/v1/worker/force-end", "application/json", nil)
	if err != nil {
		t.Fatalf("POST force-end: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST force-end = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	select {
	case code := <-exited:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	case <-time.After(time.Second):
		t.Fatal("force-end did not invoke exit")
	}
}

func TestWorkerControlRejectsWrongMethod(t *testing.T) {
	server := httptest.NewServer(newWorkerControlHandler(func(int) {}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/worker/force-end")
	if err != nil {
		t.Fatalf("GET force-end: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET force-end = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}
