package convert

import "testing"

func TestEscapeConcatPath(t *testing.T) {
	got := escapeConcatPath("a'\\b\nc\rd")
	want := "a\\'\\\\b\\\nc\\\rd"
	if got != want {
		t.Fatalf("escapeConcatPath() = %q, want %q", got, want)
	}
}
