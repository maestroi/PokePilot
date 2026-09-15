package emu

import "testing"

func TestUniformRGBFrame(t *testing.T) {
	tests := []struct {
		name string
		rgb  []byte
		want bool
	}{
		{name: "white fade", rgb: []byte{255, 255, 255, 255, 255, 255, 255, 255, 255}, want: true},
		{name: "black fade", rgb: []byte{0, 0, 0, 0, 0, 0}, want: true},
		{name: "visible frame", rgb: []byte{255, 255, 255, 255, 255, 255, 248, 248, 248}, want: false},
		{name: "short input", rgb: []byte{255, 255, 255}, want: false},
		{name: "malformed input", rgb: []byte{255, 255, 255, 255}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := uniformRGBFrame(tt.rgb); got != tt.want {
				t.Fatalf("uniformRGBFrame(%v) = %v, want %v", tt.rgb, got, tt.want)
			}
		})
	}
}
