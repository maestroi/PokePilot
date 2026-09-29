package encode

import "testing"

func TestTailWriterKeepsNewestBoundedOutput(t *testing.T) {
	w := &tailWriter{limit: 8}
	for _, chunk := range []string{"abc", "defg", "hijkl"} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := w.String(), "efghijkl"; got != want {
		t.Fatalf("tail=%q want %q", got, want)
	}
	if _, err := w.Write([]byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	if got, want := w.String(), "23456789"; got != want {
		t.Fatalf("large write tail=%q want %q", got, want)
	}
}
