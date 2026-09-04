package check_test

import (
	"reflect"
	"strings"
	"testing"

	"ergo.services/ergo/testing/check"
)

var notAFilter = map[string]bool{
	"Once": true, "Times": true, "AtLeast": true, "None": true,
	"Since": true, "Within": true, "Assert": true, "Must": true,
	"Capture": true, "Collect": true, "Where": true,
}

func zeroFilterArg(t reflect.Type) reflect.Value {
	if t.Kind() == reflect.Func {
		return reflect.MakeFunc(t, func([]reflect.Value) []reflect.Value {
			out := make([]reflect.Value, t.NumOut())
			for i := range out {
				out[i] = reflect.Zero(t.Out(i))
			}
			return out
		})
	}
	return reflect.Zero(t)
}

func callFilter(fn reflect.Value) {
	ft := fn.Type()
	n := ft.NumIn()
	if ft.IsVariadic() {
		n--
	}
	args := make([]reflect.Value, n)
	for i := 0; i < n; i++ {
		args[i] = zeroFilterArg(ft.In(i))
	}
	fn.Call(args)
}

func TestEveryAssertionBuilderIsChainable(t *testing.T) {
	asserterType := reflect.TypeOf(&check.Asserter{})

	for i := 0; i < asserterType.NumMethod(); i++ {
		entry := asserterType.Method(i)
		if strings.HasPrefix(entry.Name, "Should") == false {
			continue
		}
		t.Run(entry.Name, func(t *testing.T) {
			asserter := check.NewAsserter(t, check.NewRecorder())
			assertion := reflect.ValueOf(asserter).MethodByName(entry.Name).Call(nil)[0]
			if assertion.IsNil() {
				t.Fatalf("%s returned nothing to assert on", entry.Name)
			}

			assertionType := assertion.Type()
			filters := 0
			for j := 0; j < assertionType.NumMethod(); j++ {
				builder := assertionType.Method(j)
				if notAFilter[builder.Name] {
					continue
				}
				if builder.Type.NumOut() != 1 || builder.Type.Out(0) != assertionType {
					continue
				}
				filters++
				chained := assertion.MethodByName(builder.Name)
				callFilter(chained)
			}
			if filters == 0 {
				t.Fatalf("%s has no filter to narrow it with", entry.Name)
			}

			assertion.MethodByName("None").Call(nil)
			assertion.MethodByName("Assert").Call(nil)
		})
	}
}
