package gen

import "testing"

func TestArg(t *testing.T) {
	args := []any{"cfg", nil, 7}

	if v, ok := Arg[string](args, 0); ok == false || v != "cfg" {
		t.Fatalf("Arg[string](0): got %q %v, want cfg true", v, ok)
	}
	if _, ok := Arg[string](args, 1); ok {
		t.Fatal("Arg[string](1): a nil argument is not a string")
	}
	if _, ok := Arg[string](args, 2); ok {
		t.Fatal("Arg[string](2): an int argument is not a string")
	}
	if v, ok := Arg[int](args, 2); ok == false || v != 7 {
		t.Fatalf("Arg[int](2): got %d %v, want 7 true", v, ok)
	}
	if _, ok := Arg[int](args, 3); ok {
		t.Fatal("Arg[int](3): there is no argument 3")
	}
	if _, ok := Arg[int](args, -1); ok {
		t.Fatal("Arg[int](-1): there is no argument -1")
	}
	if _, ok := Arg[any](args, 1); ok {
		t.Fatal("Arg[any](1): a nil argument is not a value")
	}
}
