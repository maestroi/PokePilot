package artifactstore

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestDeletePrefixPaginatesAndDeletesObjects(t *testing.T) {
	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/pokepilot" {
				t.Fatalf("list path = %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("list-type"); got != "2" {
				t.Fatalf("list-type = %q, want 2", got)
			}
			if got := r.URL.Query().Get("prefix"); got != "runs/run-1/" {
				t.Fatalf("prefix = %q", got)
			}
			if r.Header.Get("Authorization") == "" {
				t.Fatal("list request was not signed")
			}
			w.Header().Set("Content-Type", "application/xml")
			if r.URL.Query().Get("continuation-token") == "" {
				fmt.Fprint(w, `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>next</NextContinuationToken><Contents><Key>runs/run-1/attempt-1/run.gbrun</Key></Contents><Contents><Key>runs/run-1/attempt-1/replay-deadbeef.mp4</Key></Contents></ListBucketResult>`)
				return
			}
			if r.URL.Query().Get("continuation-token") != "next" {
				t.Fatalf("continuation-token = %q", r.URL.Query().Get("continuation-token"))
			}
			fmt.Fprint(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>runs/run-1/attempt-2/run.gbrun</Key></Contents></ListBucketResult>`)
		case http.MethodDelete:
			if r.Header.Get("Authorization") == "" {
				t.Fatal("delete request was not signed")
			}
			if got := r.Header.Get("x-amz-content-sha256"); got != emptyPayloadHash {
				t.Fatalf("delete payload hash = %q", got)
			}
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	store, err := NewS3(S3Config{
		Endpoint: server.URL, Bucket: "pokepilot", Region: "us-east-1",
		AccessKey: "test-access", SecretKey: "test-secret", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	count, err := store.DeletePrefix(context.Background(), "/runs/run-1/")
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("deleted count = %d, want 3", count)
	}
	want := []string{
		"/pokepilot/runs/run-1/attempt-1/run.gbrun",
		"/pokepilot/runs/run-1/attempt-1/replay-deadbeef.mp4",
		"/pokepilot/runs/run-1/attempt-2/run.gbrun",
	}
	if !reflect.DeepEqual(deleted, want) {
		t.Fatalf("deleted = %#v, want %#v", deleted, want)
	}
}

func TestDeleteObjectTreatsMissingAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/pokepilot/runs/run-1/missing" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	store, err := NewS3(S3Config{
		Endpoint: server.URL, Bucket: "pokepilot", Region: "us-east-1",
		AccessKey: "test-access", SecretKey: "test-secret", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteObject(context.Background(), "runs/run-1/missing"); err != nil {
		t.Fatalf("DeleteObject missing: %v", err)
	}
}
