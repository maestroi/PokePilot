package emu

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWatchExtraRouteServesBufferedData(t *testing.T) {
	m := openTestEmu(t)
	if err := m.HandleWatch("/render-state.json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"schema_version":1}`) //nolint:errcheck // test handler
	})); err != nil {
		t.Fatalf("HandleWatch: %v", err)
	}
	addr, err := m.Watch("127.0.0.1:0", 3)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	resp, err := http.Get("http://" + addr + "/render-state.json")
	if err != nil {
		t.Fatalf("GET render state: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET render state = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q", got)
	}
	if string(body) != `{"schema_version":1}` {
		t.Fatalf("body = %q", body)
	}
}

func TestWatchExtraRouteRejectsReservedOrLateRegistration(t *testing.T) {
	m := openTestEmu(t)
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if err := m.HandleWatch("/frame.png", h); err == nil {
		t.Fatal("reserved route accepted")
	}
	if _, err := m.Watch("127.0.0.1:0", 3); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if err := m.HandleWatch("/render-state.json", h); err == nil {
		t.Fatal("late route registration accepted")
	}
}
