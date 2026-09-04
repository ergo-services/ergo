package lib

import (
	"strings"
	"testing"
)

func panicExplicit() { panic("boom") }
func panicDivide()   { a, b := 1, 0; _ = a / b }
func panicNilDeref() { var p *int; _ = *p }
func panicBounds()   { s := []int{}; _ = s[3] }
func panicNilMap()   { var m map[string]int; m["k"] = 1 }
func panicAssert()   { var v any = "s"; _ = v.(int) }

func originOf(f func()) (origin string) {
	defer func() {
		if r := recover(); r != nil {
			origin = PanicOrigin()
		}
	}()
	f()
	return
}

func TestPanicOriginNamesTheFaultingFunction(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func()
		want string
	}{
		{"explicit panic", panicExplicit, "lib.panicExplicit"},
		{"divide by zero", panicDivide, "lib.panicDivide"},
		{"nil dereference", panicNilDeref, "lib.panicNilDeref"},
		{"index out of range", panicBounds, "lib.panicBounds"},
		{"nil map write", panicNilMap, "lib.panicNilMap"},
		{"type assertion", panicAssert, "lib.panicAssert"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			origin := originOf(tc.fn)
			if strings.Contains(origin, tc.want) == false {
				t.Fatalf("the panic is reported at %s, not in %s", origin, tc.want)
			}
			if strings.Contains(origin, "panic_test.go:") == false {
				t.Fatalf("the panic is reported at %s, without the file and line it happened on", origin)
			}
			if strings.Contains(origin, "runtime.") {
				t.Fatalf("the panic is reported at %s, which is a runtime frame", origin)
			}
		})
	}
}
