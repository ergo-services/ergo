package lib

import "testing"

func TestRandomString(t *testing.T) {
	s := RandomString(16)
	if len(s) != 16 {
		t.Fatalf("RandomString(16) length = %d, want 16", len(s))
	}
	if RandomString(16) == s {
		t.Fatal("two RandomString calls returned the same value")
	}
}
