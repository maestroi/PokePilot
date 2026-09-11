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
	  {"key":"0badf00d","count":3,"example":"free","run_ids":["r2"],"issue":{"status":"open"}}
	]`)
	var out bytes.Buffer
	err := run([]string{"pick", "--claimed", "fix(farm): x [triage:cafef00d]"}, in, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"key": "0badf00d"`) && !strings.Contains(out.String(), `"key":"0badf00d"`) {
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
