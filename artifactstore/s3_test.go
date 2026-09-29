package artifactstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPutObjectUsesPathStyleAndSigV4(t *testing.T) {
	body := []byte("deterministic replay")
	sum := sha256.Sum256(body)
	wantHash := hex.EncodeToString(sum[:])

	var gotPath, gotAuth, gotDate, gotHash, gotType string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotDate = r.Header.Get("x-amz-date")
		gotHash = r.Header.Get("x-amz-content-sha256")
		gotType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store, err := NewS3(S3Config{
		Endpoint:  server.URL,
		Bucket:    "pokepilot",
		Region:    "us-east-1",
		AccessKey: "test-access",
		SecretKey: "test-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time {
		return time.Date(2026, 9, 5, 11, 22, 33, 0, time.UTC)
	}

	obj, err := store.PutObject(context.Background(), "runs/run-1/attempt-2/run.gbrun", "application/octet-stream", body)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/pokepilot/runs/run-1/attempt-2/run.gbrun" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotDate != "20260905T112233Z" {
		t.Fatalf("x-amz-date = %q", gotDate)
	}
	if gotHash != wantHash {
		t.Fatalf("payload hash = %q, want %q", gotHash, wantHash)
	}
	if gotType != "application/octet-stream" {
		t.Fatalf("content type = %q", gotType)
	}
	if string(gotBody) != string(body) {
		t.Fatalf("body = %q", gotBody)
	}
	if !strings.Contains(gotAuth, "Credential=test-access/20260905/us-east-1/s3/aws4_request") {
		t.Fatalf("authorization missing credential scope: %q", gotAuth)
	}
	if !strings.Contains(gotAuth, "SignedHeaders=host;x-amz-content-sha256;x-amz-date") {
		t.Fatalf("authorization missing signed headers: %q", gotAuth)
	}
	if obj.Bucket != "pokepilot" || obj.Key != "runs/run-1/attempt-2/run.gbrun" || obj.Size != int64(len(body)) || obj.SHA256 != wantHash {
		t.Fatalf("object = %+v", obj)
	}
}

func TestS3FromEnvDisabledWhenUnset(t *testing.T) {
	for _, key := range []string{EnvS3Endpoint, EnvS3Bucket, EnvS3Region, EnvS3AccessKey, EnvS3SecretKey, EnvS3Timeout} {
		t.Setenv(key, "")
	}
	store, configured, err := S3FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if configured || store != nil {
		t.Fatalf("store=%v configured=%v, want disabled", store, configured)
	}
}

func TestS3FromEnvOptionalDefaultsDoNotEnableStorage(t *testing.T) {
	t.Setenv(EnvS3Endpoint, "")
	t.Setenv(EnvS3Bucket, "")
	t.Setenv(EnvS3AccessKey, "")
	t.Setenv(EnvS3SecretKey, "")
	t.Setenv(EnvS3Region, "us-east-1")
	t.Setenv(EnvS3Timeout, "60s")
	store, configured, err := S3FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if configured || store != nil {
		t.Fatalf("optional defaults enabled storage: store=%v configured=%v", store, configured)
	}
}

func TestS3FromEnvRejectsPartialConfig(t *testing.T) {
	t.Setenv(EnvS3Endpoint, "http://nas:9000")
	t.Setenv(EnvS3Bucket, "pokepilot")
	t.Setenv(EnvS3AccessKey, "")
	t.Setenv(EnvS3SecretKey, "")
	_, configured, err := S3FromEnv()
	if !configured {
		t.Fatal("partial config should count as configured")
	}
	if err == nil || !strings.Contains(err.Error(), EnvS3AccessKey) {
		t.Fatalf("error = %v, want missing access key", err)
	}
}

func TestPresignGetObjectIsDeterministicAndScoped(t *testing.T) {
	store, err := NewS3(S3Config{
		Endpoint:  "https://objects.example.test:9443/base",
		Bucket:    "pokepilot",
		Region:    "eu-west-1",
		AccessKey: "test-access",
		SecretKey: "test-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time {
		return time.Date(2026, 9, 29, 7, 30, 0, 0, time.UTC)
	}

	got, err := store.PresignGetObject("runs/run-1/segment 01.mp4", 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "objects.example.test:9443" {
		t.Fatalf("url host=%s://%s", u.Scheme, u.Host)
	}
	if u.Path != "/base/pokepilot/runs/run-1/segment 01.mp4" {
		t.Fatalf("path=%q", u.Path)
	}
	q := u.Query()
	if q.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
		t.Fatalf("algorithm=%q", q.Get("X-Amz-Algorithm"))
	}
	if q.Get("X-Amz-Credential") != "test-access/20260929/eu-west-1/s3/aws4_request" {
		t.Fatalf("credential=%q", q.Get("X-Amz-Credential"))
	}
	if q.Get("X-Amz-Date") != "20260929T073000Z" || q.Get("X-Amz-Expires") != "7200" {
		t.Fatalf("date/expires=%q/%q", q.Get("X-Amz-Date"), q.Get("X-Amz-Expires"))
	}
	if q.Get("X-Amz-SignedHeaders") != "host" || q.Get("X-Amz-Signature") == "" {
		t.Fatalf("signed headers/signature=%q/%q", q.Get("X-Amz-SignedHeaders"), q.Get("X-Amz-Signature"))
	}

	again, err := store.PresignGetObject("runs/run-1/segment 01.mp4", 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Fatalf("presigned URL changed for same inputs:\n%s\n%s", got, again)
	}
}

func TestPresignGetObjectRejectsInvalidTTL(t *testing.T) {
	store, err := NewS3(S3Config{
		Endpoint: "https://objects.example.test", Bucket: "pokepilot", AccessKey: "a", SecretKey: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ttl := range []time.Duration{0, -time.Second, 8 * 24 * time.Hour} {
		if _, err := store.PresignGetObject("x", ttl); err == nil {
			t.Fatalf("ttl %s unexpectedly accepted", ttl)
		}
	}
}
