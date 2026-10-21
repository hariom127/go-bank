package db

import "testing"

func TestAdd(t *testing.T) {
	got := 2 + 4
	want := 5

	if got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}
