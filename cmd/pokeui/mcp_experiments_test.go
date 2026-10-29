package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestMCPExperimentsAndModels(t *testing.T) {
	var created farm.ExperimentRequest
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		switch req.Method + " " + req.URL.Path {
		case "GET /v1/experiments":
			res.Write([]byte(`{"experiments":[{"id":"e2","name":"new","arm_a":{"runs":4,"success_rate":0.5,"replan_reasons":{"x":1}},"pairs":[{"seed":1}]},{"id":"e1"}]}`)) //nolint:errcheck
		case "POST /v1/experiments":
			json.NewDecoder(req.Body).Decode(&created) //nolint:errcheck
			res.WriteHeader(http.StatusCreated)
			res.Write([]byte(`{"id":"e3","name":"made","total_pairs":2}`)) //nolint:errcheck
		case "GET /v1/models":
			res.Write([]byte(`{"deployments":[{"id":"gpu-a","label":"Qwen","state":"ready","control_url":"http://host:1","token_env":"SECRET_ENV","endpoint":"http://host:2"}]}`)) //nolint:errcheck
		default:
			http.NotFound(res, req)
		}
	}))
	defer wall.Close()
	control := &mcpControl{wallBase: wall.URL, http: wall.Client()}
	ctx := context.Background()

	_, list, err := control.listExperiments(ctx, nil, mcpExperimentsInput{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	experiments := list["experiments"].([]map[string]any)
	if len(experiments) != 1 || list["total"] != 2 || experiments[0]["id"] != "e2" {
		t.Fatalf("list = %#v", list)
	}
	if _, ok := experiments[0]["pairs"]; ok {
		t.Fatal("summary carries per-seed pairs")
	}
	if arm := experiments[0]["arm_a"].(map[string]any); arm["success_rate"] != 0.5 || arm["replan_reasons"] != nil {
		t.Fatalf("arm_a = %#v", arm)
	}

	_, started, err := control.startExperiment(ctx, nil, farm.ExperimentRequest{Name: "made", Goal: "badges:1", SeedCount: 2,
		ArmA: farm.ExperimentArm{Deployment: "gpu-a"}, ArmB: farm.ExperimentArm{Deployment: "gpu-b"}})
	if err != nil || started["id"] != "e3" || created.ArmB.Deployment != "gpu-b" {
		t.Fatalf("start = %#v, %v; wall got %+v", started, err, created)
	}

	_, models, err := control.listModels(ctx, nil, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(models)
	if !strings.Contains(string(data), `"gpu-a"`) || strings.Contains(string(data), "host:") || strings.Contains(string(data), "SECRET_ENV") {
		t.Fatalf("models = %s", data)
	}
}
