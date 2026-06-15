package local

import (
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// t1proc verifies its own gen.Process API on demand. Value getters return the
// value (the test asserts it); multi-step procedures run in-process and return an
// error (nil means the check passed).
type t1proc struct{ act.Actor }

func factoryT1Proc() gen.ProcessBehavior { return &t1proc{} }

func (p *t1proc) Init(args ...any) error {
	for k, v := range args[0].(map[gen.Env]any) {
		p.SetEnv(k, v)
	}
	return nil
}

func (p *t1proc) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request.(string) {
	case "node":
		return p.Node(), nil
	case "pid":
		return p.PID(), nil
	case "name":
		return p.Name(), nil
	case "parent":
		return p.Parent(), nil
	case "leader":
		return p.Leader(), nil
	case "uptime":
		return p.Uptime(), nil
	case "state":
		return p.State(), nil
	case "env":
		return p.EnvList(), nil
	case "envproc":
		return errText(p.checkEnv()), nil
	case "nameproc":
		return errText(p.checkName()), nil
	case "compression":
		return errText(p.checkCompression()), nil
	case "sendpriority":
		return errText(p.checkSendPriority()), nil
	case "aliases":
		return errText(p.checkAliases()), nil
	case "events":
		return errText(p.checkEvents()), nil
	case "spawn":
		return errText(p.checkSpawn()), nil
	}
	return "unknown check", nil
}

// errText returns "" for nil (so the Call gets a non-nil response: returning a
// nil result would be treated by act.Actor as a deferred reply).
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (p *t1proc) checkEnv() error {
	p.SetEnv("k", int(123))
	if v, exist := p.Env("k"); exist == false {
		return fmt.Errorf("env k missing")
	} else if i, _ := v.(int); i != 123 {
		return fmt.Errorf("env k != 123")
	}
	p.SetEnv("k", nil)
	if _, exist := p.Env("k"); exist {
		return fmt.Errorf("env k not removed")
	}
	if reflect.DeepEqual(p.EnvList(), p.Node().EnvList()) {
		return fmt.Errorf("process env equals node env")
	}
	return nil
}

func (p *t1proc) checkName() error {
	if err := p.RegisterName("newname"); err != gen.ErrTaken {
		return fmt.Errorf("RegisterName while named: got %v want ErrTaken", err)
	}
	if err := p.UnregisterName(); err != nil {
		return err
	}
	if err := p.UnregisterName(); err != gen.ErrNameUnknown {
		return fmt.Errorf("UnregisterName twice: got %v want ErrNameUnknown", err)
	}
	if err := p.RegisterName("newname"); err != nil {
		return err
	}
	if p.Name() != "newname" {
		return fmt.Errorf("Name != newname")
	}
	return nil
}

func (p *t1proc) checkCompression() error {
	if p.Compression() {
		return fmt.Errorf("compression on by default")
	}
	if err := p.SetCompression(true); err != nil {
		return err
	}
	if p.Compression() == false {
		return fmt.Errorf("compression not enabled")
	}
	if p.CompressionLevel() != gen.DefaultCompressionLevel {
		return fmt.Errorf("bad default level")
	}
	if err := p.SetCompressionLevel(100); err != gen.ErrIncorrect {
		return fmt.Errorf("SetCompressionLevel(100): got %v want ErrIncorrect", err)
	}
	if err := p.SetCompressionLevel(gen.CompressionBestSize); err != nil {
		return err
	}
	if p.CompressionLevel() != gen.CompressionBestSize {
		return fmt.Errorf("level not set")
	}
	if p.CompressionThreshold() != gen.DefaultCompressionThreshold {
		return fmt.Errorf("bad default threshold")
	}
	if err := p.SetCompressionThreshold(1); err != gen.ErrIncorrect {
		return fmt.Errorf("SetCompressionThreshold(1): got %v want ErrIncorrect", err)
	}
	if err := p.SetCompressionThreshold(gen.DefaultCompressionThreshold + 100); err != nil {
		return err
	}
	if p.CompressionThreshold() != gen.DefaultCompressionThreshold+100 {
		return fmt.Errorf("threshold not set")
	}
	return nil
}

func (p *t1proc) checkSendPriority() error {
	if p.SendPriority() != gen.MessagePriorityNormal {
		return fmt.Errorf("bad default priority")
	}
	if err := p.SetSendPriority(gen.MessagePriorityMax); err != nil {
		return err
	}
	if p.SendPriority() != gen.MessagePriorityMax {
		return fmt.Errorf("priority not set")
	}
	if err := p.SetSendPriority(gen.MessagePriority(12345)); err != gen.ErrIncorrect {
		return fmt.Errorf("SetSendPriority(bad): got %v want ErrIncorrect", err)
	}
	return nil
}

func (p *t1proc) checkAliases() error {
	if len(p.Aliases()) != 0 {
		return fmt.Errorf("aliases not empty")
	}
	a1, err := p.CreateAlias()
	if err != nil {
		return err
	}
	a2, err := p.CreateAlias()
	if err != nil {
		return err
	}
	if reflect.DeepEqual([]gen.Alias{a1, a2}, p.Aliases()) == false {
		return fmt.Errorf("aliases list mismatch after create")
	}
	if err := p.DeleteAlias(a1); err != nil {
		return err
	}
	if reflect.DeepEqual([]gen.Alias{a2}, p.Aliases()) == false {
		return fmt.Errorf("aliases list mismatch after delete")
	}
	return nil
}

func (p *t1proc) checkEvents() error {
	e1, e2 := gen.Atom("e1"), gen.Atom("e2")
	if len(p.Events()) != 0 {
		return fmt.Errorf("events not empty")
	}
	opts := gen.EventOptions{Notify: true, Buffer: 10}
	if _, err := p.RegisterEvent(e1, opts); err != nil {
		return err
	}
	if _, err := p.RegisterEvent(e2, opts); err != nil {
		return err
	}
	got := map[gen.Atom]bool{}
	for _, e := range p.Events() {
		got[e] = true
	}
	if reflect.DeepEqual(map[gen.Atom]bool{e1: true, e2: true}, got) == false {
		return fmt.Errorf("events list mismatch")
	}
	if err := p.UnregisterEvent(e1); err != nil {
		return err
	}
	if reflect.DeepEqual([]gen.Atom{e2}, p.Events()) == false {
		return fmt.Errorf("events list mismatch after unregister")
	}
	return nil
}

func (p *t1proc) checkSpawn() error {
	factory := func() gen.ProcessBehavior {
		x := struct{ act.Actor }{}
		return &x
	}
	pid, err := p.Spawn(factory, gen.ProcessOptions{})
	if err != nil {
		return err
	}
	info, err := p.Node().ProcessInfo(pid)
	if err != nil {
		return err
	}
	if info.Parent != p.PID() {
		return fmt.Errorf("spawned child parent mismatch")
	}
	pid, err = p.SpawnRegister("reg", factory, gen.ProcessOptions{})
	if err != nil {
		return err
	}
	info, err = p.Node().ProcessInfo(pid)
	if err != nil {
		return err
	}
	if info.Parent != p.PID() {
		return fmt.Errorf("spawn-register child parent mismatch")
	}
	if info.Name != "reg" {
		return fmt.Errorf("spawn-register child name mismatch")
	}
	return nil
}

// TestLocalProcess: a process exposes its identity, env, name, compression,
// send-priority, aliases, events, spawning and lifecycle via the gen.Process API.
func TestLocalProcess(t *testing.T) {
	nenv := map[gen.Env]any{gen.Env("A"): 1, gen.Env("B"): 1.23, gen.Env("C"): "d"}
	penv := map[gen.Env]any{gen.Env("B"): 1.23, gen.Env("D"): "d"}
	expEnv := map[gen.Env]any{}
	for k, v := range nenv {
		expEnv[k] = v
	}
	for k, v := range penv {
		expEnv[k] = v
	}

	s := stage.New(t)
	n := s.Node("n", stage.NodeOptions{Env: nenv})
	p := n.SpawnRegister("a", factoryT1Proc, gen.ProcessOptions{}, penv)

	get := func(req string) any {
		t.Helper()
		v, err := n.Call(p, req)
		check.NoError(t, err)
		return v
	}
	proc := func(req string) {
		t.Helper()
		v, err := n.Call(p, req)
		check.NoError(t, err)
		check.Equal(t, "", v)
	}

	check.True(t, get("node") == n.Native())
	check.Equal(t, p, get("pid"))
	check.Equal(t, gen.Atom("a"), get("name"))
	check.Equal(t, n.PID(), get("parent"))
	check.Equal(t, n.PID(), get("leader"))
	check.Equal(t, int64(0), get("uptime"))
	check.Equal(t, gen.ProcessStateRunning, get("state"))
	check.Equal(t, expEnv, get("env"))

	proc("envproc")
	proc("compression")
	proc("sendpriority")
	proc("aliases")
	proc("events")
	proc("spawn")
	proc("nameproc") // last: it changes the registered name
}
