package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPickCLI(t *testing.T) {
	in := strings.NewReader(`[
	  {"key":"cafef00d","count":4,"example":"claimed","run_ids":["r1"],"issue":{"status":"open"}},
	  {"key":"0badf00d","count":3,"example":"free","run_ids":["r2"],"issue":{"issue_number":432,"status":"open"}}
	]`)
	var out bytes.Buffer
	err := run([]string{"pick", "--claimed", "fix(farm): x [triage:cafef00d]"}, in, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"key": "0badf00d"`) && !strings.Contains(out.String(), `"key":"0badf00d"`) {
		t.Fatalf("output = %s", out.String())
	}
	if !strings.Contains(out.String(), `"issue_number": 432`) && !strings.Contains(out.String(), `"issue_number":432`) {
		t.Fatalf("output did not preserve linked issue number: %s", out.String())
	}
}

func TestPickCLILocalRepairAndRegression(t *testing.T) {
	in := strings.NewReader(`[
	  {"key":"fixed","count":9,"example":"old","run_ids":["old"]},
	  {"key":"regressed","count":4,"example":"again","run_ids":["new"],"issue":{"status":"resolved","resolution":"fixed"}}
	]`)
	var out bytes.Buffer
	err := run([]string{"pick", "--repaired", "fixed", "--regressed", "regressed"}, in, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"key": "regressed"`) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestPickCLINothingFree(t *testing.T) {
	in := strings.NewReader(`[
	  {"key":"cafef00d","count":4,"example":"claimed","run_ids":["r1"],"issue":{"status":"resolved","resolution":"fixed"}}
	]`)
	err := run([]string{"pick"}, in, &bytes.Buffer{})
	if !errors.Is(err, errNothing) {
		t.Fatalf("err = %v, want errNothing", err)
	}
}
