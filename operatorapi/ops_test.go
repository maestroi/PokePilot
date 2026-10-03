package operatorapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPostOpsCarriesBearerAndBody(t *testing.T) {
	var got NodeReport
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !OpsAuthorized(r, "s3cret") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()
	want := NodeReport{Node: "n1", At: 7, Disks: []DiskFree{{Mount: "/", FreeGB: 12}}}
	if err := PostOps(context.Background(), srv.Client(), srv.URL, "s3cret", want); err != nil {
		t.Fatal(err)
	}
	if got.Node != "n1" || got.Disks[0].FreeGB != 12 {
		t.Fatalf("got %+v", got)
	}
	if err := PostOps(context.Background(), srv.Client(), srv.URL, "wrong", want); err == nil {
		t.Fatal("wrong token must fail")
	}
}

func TestOpsAuthorizedRejectsEmptyToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("Authorization", "Bearer ")
	if OpsAuthorized(r, "") {
		t.Fatal("an unset token must never authorize")
	}
}

func TestReadSecretFileTrims(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tok")
	_ = os.WriteFile(p, []byte("abc\n"), 0o600)
	if ReadSecretFile(p) != "abc" || ReadSecretFile("") != "" {
		t.Fatal("ReadSecretFile")
	}
}
