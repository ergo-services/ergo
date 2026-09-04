package mock_test

import (
	"reflect"
	"strings"
	"testing"

	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

type mockKind struct {
	name string
	make func() any
}

var mockKinds = []mockKind{
	{"Node", func() any { return mock.NewNode() }},
	{"Process", func() any { return mock.NewProcess() }},
	{"Meta", func() any { return mock.NewMeta() }},
	{"Log", func() any { return mock.NewLog() }},
	{"Cron", func() any { return mock.NewCron() }},
	{"Network", func() any { return mock.NewNetwork() }},
	{"RemoteNode", func() any { return mock.NewRemoteNode() }},
	{"Registrar", func() any { return mock.NewRegistrar() }},
	{"Resolver", func() any { return mock.NewResolver() }},
	{"Core", func() any { return mock.NewCore() }},
	{"CoreTargetManager", func() any { return mock.NewCoreTargetManager() }},
	{"Connection", func() any { return mock.NewConnection() }},
}

func asserterMethods() map[string]bool {
	names := map[string]bool{}
	t := reflect.TypeOf(&check.Asserter{})
	for i := 0; i < t.NumMethod(); i++ {
		names[t.Method(i).Name] = true
	}
	return names
}

func interfaceMethods(v any) []reflect.Method {
	promoted := asserterMethods()
	t := reflect.TypeOf(v)
	var out []reflect.Method
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if strings.HasPrefix(m.Name, "On") || promoted[m.Name] {
			continue
		}
		out = append(out, m)
	}
	return out
}

func argFor(t reflect.Type) reflect.Value {
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

func callWithZeroArgs(fn reflect.Value) []reflect.Value {
	ft := fn.Type()
	n := ft.NumIn()
	if ft.IsVariadic() {
		n--
	}
	args := make([]reflect.Value, n)
	for i := 0; i < n; i++ {
		args[i] = argFor(ft.In(i))
	}
	return fn.Call(args)
}

func TestEveryMockMethodIsSafeUnstubbed(t *testing.T) {
	for _, kind := range mockKinds {
		for _, m := range interfaceMethods(kind.make()) {
			t.Run(kind.name+"/"+m.Name, func(t *testing.T) {
				callWithZeroArgs(reflect.ValueOf(kind.make()).MethodByName(m.Name))
			})
		}
	}
}

func TestEveryMockMethodHasAnOverrideSetter(t *testing.T) {
	for _, kind := range mockKinds {
		v := kind.make()
		rt := reflect.TypeOf(v)
		for _, m := range interfaceMethods(v) {
			setter, found := rt.MethodByName("On" + m.Name)
			if found == false {
				t.Errorf("%s.%s has no On%s setter", kind.name, m.Name, m.Name)
				continue
			}
			if setter.Type.NumIn() != 2 || setter.Type.In(1).Kind() != reflect.Func {
				t.Errorf("%s.On%s does not take a single function", kind.name, m.Name)
			}
		}
	}
}

func TestEveryOverrideSetterRedirectsItsMethod(t *testing.T) {
	for _, kind := range mockKinds {
		rt := reflect.TypeOf(kind.make())
		for _, m := range interfaceMethods(kind.make()) {
			setter, found := rt.MethodByName("On" + m.Name)
			if found == false {
				continue
			}
			if setter.Type.NumIn() != 2 || setter.Type.In(1).Kind() != reflect.Func {
				continue
			}
			t.Run(kind.name+"/"+m.Name, func(t *testing.T) {
				subject := kind.make()
				called := false
				ft := setter.Type.In(1)
				override := reflect.MakeFunc(ft, func([]reflect.Value) []reflect.Value {
					called = true
					out := make([]reflect.Value, ft.NumOut())
					for i := range out {
						out[i] = reflect.Zero(ft.Out(i))
					}
					return out
				})
				reflect.ValueOf(subject).MethodByName("On" + m.Name).Call([]reflect.Value{override})
				callWithZeroArgs(reflect.ValueOf(subject).MethodByName(m.Name))
				if called == false {
					t.Errorf("On%s was set and %s.%s did not use it", m.Name, kind.name, m.Name)
				}
			})
		}
	}
}
