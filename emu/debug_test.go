package emu

import "testing"

func TestIsCheckedState(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want bool
	}{
		{name: "checked", data: []byte("GMBSTATEpayload"), want: true},
		{name: "magic only", data: []byte("GMBSTATE"), want: true},
		{name: "raw", data: []byte("raw-gob-state"), want: false},
		{name: "prefix too short", data: []byte("GMBSTAT"), want: false},
		{name: "empty", data: nil, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCheckedState(tc.data); got != tc.want {
				t.Fatalf("isCheckedState(%q) = %v, want %v", tc.data, got, tc.want)
			}
		})
	}
}
