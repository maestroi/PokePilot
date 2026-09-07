package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTransportForStampsBearer(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	for _, tc := range []struct{ token, want string }{
		{"abc123", "Bearer abc123"},
		{"Bearer abc123", "Bearer abc123"}, // already-prefixed tokens are not double-stamped
		{"  ", ""},                         // no token: unauthenticated, as before
	} {
		got = ""
		client := &http.Client{Transport: transportFor(tc.token)}
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		resp.Body.Close()
		if got != tc.want {
			t.Errorf("token %q: Authorization = %q, want %q", tc.token, got, tc.want)
		}
	}
}
