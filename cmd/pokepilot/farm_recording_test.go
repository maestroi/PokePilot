package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/farm"
)

func TestFarmRecordingMetadata(t *testing.T) {
	spec := farm.Spec{
		RunID:      "run-42",
		Attempt:    3,
		Seed:       12345,
		LLMProfile: "primary",
	}
	got := farmRecordingMetadata(spec, "llm", "squirtle", "", "earn the Boulder Badge", 87, "abc123")
	want := map[string]string{
		"run_id":         "run-42",
		"attempt":        "3",
		"runner_version": "abc123",
		"planner":        "llm",
		"seed":           "12345",
		"seed_burn":      "87",
		"starter":        "squirtle",
		"goal":           "earn the Boulder Badge",
		"llm_profile":    "primary",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata = %#v, want %#v", got, want)
	}
	if _, ok := got["dest"]; ok {
		t.Fatal("empty destination must be omitted")
	}
}

func TestFarmRecordingArtifactValidates(t *testing.T) {
	data := []byte("deterministic gbrun bytes")
	art := farmRecordingArtifact(data)
	if art.Name != farmRecordingName {
		t.Fatalf("artifact name = %q, want %q", art.Name, farmRecordingName)
	}
	if art.MediaType != "application/octet-stream" {
		t.Fatalf("media type = %q", art.MediaType)
	}
	if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: []farm.Artifact{art}}); err != nil {
		t.Fatalf("recording artifact should pass farm validation: %v", err)
	}
}

func TestFarmRecordingObjectKeyIsBrowsableAndStable(t *testing.T) {
	spec := farm.Spec{RunID: "badge farm / weird run", Attempt: 4}
	got := farmRecordingObjectKey(spec)
	if !strings.HasPrefix(got, "runs/badge-farm-weird-run-") {
		t.Fatalf("object key = %q", got)
	}
	if !strings.HasSuffix(got, "/attempt-4/run.gbrun") {
		t.Fatalf("object key = %q", got)
	}
	if got != farmRecordingObjectKey(spec) {
		t.Fatal("object key must be deterministic")
	}
}

func TestUploadFarmRecordingToS3ReturnsRemoteArtifact(t *testing.T) {
	data := []byte("a replay that should live on the NAS")
	var gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv(artifactstore.EnvS3Endpoint, server.URL)
	t.Setenv(artifactstore.EnvS3Bucket, "pokepilot")
	t.Setenv(artifactstore.EnvS3Region, "us-east-1")
	t.Setenv(artifactstore.EnvS3AccessKey, "example-access-key")
	t.Setenv(artifactstore.EnvS3SecretKey, "example-secret-key")
	t.Setenv(artifactstore.EnvS3Timeout, "5s")

	spec := farm.Spec{RunID: "run-42", Attempt: 2}
	art, configured, err := uploadFarmRecording(spec, data)
	if err != nil {
		t.Fatal(err)
	}
	if !configured {
		t.Fatal("S3 config was ignored")
	}
	if art.Store != farm.ArtifactStoreS3 || art.Bucket != "pokepilot" || art.Size != int64(len(data)) {
		t.Fatalf("artifact = %+v", art)
	}
	if len(art.Data) != 0 {
		t.Fatal("remote artifact must not retain inline bytes")
	}
	if gotPath != "/pokepilot/"+art.ObjectKey {
		t.Fatalf("request path = %q, object key = %q", gotPath, art.ObjectKey)
	}
	if string(gotBody) != string(data) {
		t.Fatalf("uploaded body = %q", gotBody)
	}
	if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: []farm.Artifact{art}}); err != nil {
		t.Fatalf("remote artifact should pass validation: %v", err)
	}
}
