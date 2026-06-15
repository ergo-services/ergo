package lib

import "testing"

func TestHashString64(t *testing.T) {
	if HashString64("worker") != HashString64("worker") {
		t.Fatal("HashString64 is not deterministic")
	}
	if HashString64("a") == HashString64("b") {
		t.Fatal("HashString64 collided on distinct short strings")
	}
	// FNV-1a offset basis: the empty string hashes to the seed
	if HashString64("") != 14695981039346656037 {
		t.Fatalf("HashString64(\"\") = %d, want the FNV offset basis", HashString64(""))
	}
}
