package main

import (
	"bytes"
	"testing"
)

func TestAgentTraceKind(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{`llm: 6 offered, 260ms, reply "1" -> take a starter`, "llm"},
		{`round 1: take a starter -> map 00 at (5,6)`, "round"},
		{"planner: llm — the model picks from 6 offered objectives", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := agentTraceKind(c.line); got != c.want {
			t.Errorf("agentTraceKind(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestAgentTraceLogBuffersAcrossWrites(t *testing.T) {
	var out bytes.Buffer
	var notes [][2]string
	l := &agentTraceLog{
		w: &out,
		note: func(kind, text string) {
			notes = append(notes, [2]string{kind, text})
		},
	}
	chunks := []string{
		"  ll",
		`m: 2 offered, 10ms, reply "1" -> take a starter`,
		"\n",
		"rou",
		"nd 1: take a starter -> map 00 at (1,1)\n",
		"noise without newline",
	}
	for _, c := range chunks {
		if _, err := l.Write([]byte(c)); err != nil {
			t.Fatal(err)
		}
	}
	if len(notes) != 2 {
		t.Fatalf("notes = %#v, want 2 entries", notes)
	}
	if notes[0][0] != "llm" || !bytes.Contains([]byte(notes[0][1]), []byte("take a starter")) {
		t.Fatalf("first note = %#v", notes[0])
	}
	if notes[1][0] != "round" || !bytes.Contains([]byte(notes[1][1]), []byte("round 1:")) {
		t.Fatalf("second note = %#v", notes[1])
	}
	if !bytes.Contains(out.Bytes(), []byte("llm:")) || !bytes.Contains(out.Bytes(), []byte("round 1:")) {
		t.Fatalf("stdout missing mirrored lines:\n%s", out.String())
	}
}
