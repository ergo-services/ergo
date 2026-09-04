package unit_test

import (
	"reflect"
	"strings"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

func zeroArgFor(t reflect.Type) reflect.Value {
	if t.Kind() == reflect.Ptr {
		return reflect.New(t.Elem())
	}
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

func callZero(fn reflect.Value) {
	ft := fn.Type()
	n := ft.NumIn()
	if ft.IsVariadic() {
		n--
	}
	args := make([]reflect.Value, n)
	for i := 0; i < n; i++ {
		args[i] = zeroArgFor(ft.In(i))
	}
	fn.Call(args)
}

func zeroReturning(ft reflect.Type, called *bool) reflect.Value {
	return reflect.MakeFunc(ft, func([]reflect.Value) []reflect.Value {
		*called = true
		out := make([]reflect.Value, ft.NumOut())
		for i := range out {
			out[i] = reflect.Zero(ft.Out(i))
		}
		return out
	})
}

func overrideSetters(v any, hasBase func(name string) bool) []reflect.Method {
	t := reflect.TypeOf(v)
	var out []reflect.Method
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if strings.HasPrefix(m.Name, "On") == false {
			continue
		}
		if m.Type.NumIn() != 2 || m.Type.NumOut() != 0 || m.Type.In(1).Kind() != reflect.Func {
			continue
		}
		if hasBase(strings.TrimPrefix(m.Name, "On")) == false {
			continue
		}
		out = append(out, m)
	}
	return out
}

func hasMethod(v any) func(string) bool {
	t := reflect.TypeOf(v)
	return func(name string) bool {
		_, found := t.MethodByName(name)
		return found
	}
}

var shadowedByMockNode = map[string]bool{"Spawn": true, "SpawnRegister": true, "Network": true}

var drivesTheRunLoop = map[string]bool{"State": true}

func TestMockNodeOverridesAreUsed(t *testing.T) {
	node := unit.StartNode(t, "unit@localhost", gen.NodeOptions{})
	for _, setter := range overrideSetters(node, hasMethod(node)) {
		name := strings.TrimPrefix(setter.Name, "On")
		if shadowedByMockNode[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			node := unit.StartNode(t, "unit@localhost", gen.NodeOptions{})
			called := false

			reflect.ValueOf(node).MethodByName(setter.Name).Call(
				[]reflect.Value{zeroReturning(setter.Type.In(1), &called)})
			callZero(reflect.ValueOf(node).MethodByName(name))

			if called == false {
				t.Errorf("%s was set and MockNode.%s did not use it", setter.Name, name)
			}
		})
	}
}

type reflectProbe struct{ act.Actor }

func factoryReflectProbe() gen.ProcessBehavior { return &reflectProbe{} }

type probeCall struct {
	Target string
	Method string
}

func (a *reflectProbe) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if name, ok := request.(string); ok {
		request = probeCall{Target: "process", Method: name}
	}
	c, ok := request.(probeCall)
	if ok == false {
		return "not a probe call", nil
	}

	var target any = a.Process
	if c.Target == "network" {
		target = a.Node().Network()
	}

	method := reflect.ValueOf(target).MethodByName(c.Method)
	if method.IsValid() == false {
		return "no such method", nil
	}
	callZero(method)
	return "ok", nil
}

func TestSubjectOverridesAreUsed(t *testing.T) {
	sample, err := unit.StartNode(t, "unit@localhost", gen.NodeOptions{}).Spawn(factoryReflectProbe, gen.ProcessOptions{})
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}

	for _, setter := range overrideSetters(sample, hasMethod(sample.Behavior().(*reflectProbe).Process)) {
		name := strings.TrimPrefix(setter.Name, "On")
		if drivesTheRunLoop[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			sub, err := unit.StartNode(t, "unit@localhost", gen.NodeOptions{}).Spawn(factoryReflectProbe, gen.ProcessOptions{})
			if err != nil {
				t.Fatalf("spawn: %s", err)
			}
			called := false

			reflect.ValueOf(sub).MethodByName(setter.Name).Call(
				[]reflect.Value{zeroReturning(setter.Type.In(1), &called)})

			answer, err := sub.Call(gen.PID{}, name)
			if err != nil {
				t.Fatalf("call %s: %s", name, err)
			}
			if answer != "ok" {
				t.Fatalf("the probe answered %v for %s", answer, name)
			}
			if called == false {
				t.Errorf("%s was set and the process method %s did not use it", setter.Name, name)
			}
		})
	}
}

var loudUnstubbed = map[string]bool{
	"Call": true, "CallWithTimeout": true, "CallWithPriority": true, "CallImportant": true,
	"CallPID": true, "CallProcessID": true, "CallAlias": true,
	"Inspect": true, "InspectMeta": true, "Info": true, "ShortInfo": true, "MetaInfo": true,
	"EventInfo": true, "ProcessListShortInfo": true, "ApplicationInfo": true,
	"ApplicationProcessList": true, "ApplicationProcessListShortInfo": true,
	"State": true, "Spawn": true, "SpawnMeta": true, "RemoteSpawn": true,
}

func probeMethods(v any) []string {
	t := reflect.TypeOf(v)
	var names []string
	for i := 0; i < t.NumMethod(); i++ {
		name := t.Method(i).Name
		if loudUnstubbed[name] {
			continue
		}
		names = append(names, name)
	}
	return names
}

func spawnProbe(t *testing.T) *unit.Subject {
	t.Helper()
	sub, err := unit.StartNode(t, "unit@localhost", gen.NodeOptions{}).Spawn(factoryReflectProbe, gen.ProcessOptions{})
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}
	return sub
}

func TestUnstubbedProcessMethodsAnswerADefault(t *testing.T) {
	var sample gen.Process = spawnProbe(t).Behavior().(*reflectProbe).Process

	for _, name := range probeMethods(sample) {
		t.Run(name, func(t *testing.T) {
			sub := spawnProbe(t)
			answer, err := sub.Call(gen.PID{}, probeCall{Target: "process", Method: name})
			if err != nil {
				t.Fatalf("call %s: %s", name, err)
			}
			if answer != "ok" {
				t.Fatalf("the probe answered %v for %s", answer, name)
			}
		})
	}
}

func TestUnstubbedNetworkMethodsAnswerADefault(t *testing.T) {
	sample := spawnProbe(t).Behavior().(*reflectProbe).Node().Network()

	for _, name := range probeMethods(sample) {
		t.Run(name, func(t *testing.T) {
			sub := spawnProbe(t)
			sub.Node().Network().Registrar().Resolver().OnResolve("")
			sub.Node().Network().Registrar().Resolver().OnResolveApplication("")

			answer, err := sub.Call(gen.PID{}, probeCall{Target: "network", Method: name})
			if err != nil {
				t.Fatalf("call %s: %s", name, err)
			}
			if answer != "ok" {
				t.Fatalf("the probe answered %v for %s", answer, name)
			}
		})
	}
}
