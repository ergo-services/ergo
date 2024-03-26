package local

import (
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

var (
	t1cases []*testcase
)

func factory_t1() gen.ProcessBehavior {
	return &t1{}
}

type t1 struct {
	act.Actor
}

func (t *t1) Init(args ...any) error {
	return nil
}

func (t *t1) HandleMessage(from gen.PID, message any) error {
	tc := message.(*testcase)
	// get method by name
	method := reflect.ValueOf(t).MethodByName(tc.name)
	// call this method with the provided *testcase
	args := []reflect.Value{reflect.ValueOf(tc)}
	method.Call(args)
	return nil
}

//
// test methods
//

func (t *t1) TestNodeInterface(tc *testcase) {
	if t.Node() != tc.output.(gen.Node) {
		tc.err <- errIncorrect
		return
	}
	tc.err <- nil
}

func (t *t1) TestEnv(tc *testcase) {
	if reflect.DeepEqual(t.EnvList(), tc.output) == false {
		tc.err <- errIncorrect
		return
	}
	t.SetEnv("k", int(123))
	if v, exist := t.Env("k"); exist == false {
		tc.err <- errIncorrect
		return
	} else {
		i, _ := v.(int)
		if i != 123 {
			tc.err <- errIncorrect
			return
		}
	}
	// remove env
	t.SetEnv("a", nil)
	if _, exist := t.Env("a"); exist {
		tc.err <- errIncorrect
		return
	}
	// modification of process' env shouldn't reflect on the node's env
	if reflect.DeepEqual(t.EnvList(), t.Node().EnvList()) == true {
		tc.err <- errIncorrect
		return
	}
	tc.err <- nil
}

func (t *t1) TestName(tc *testcase) {
	if reflect.DeepEqual(t.Name(), tc.output) == false {
		tc.err <- errIncorrect
		return
	}
	if err := t.RegisterName("newname"); err != gen.ErrTaken {
		tc.err <- errIncorrect
		return
	}

	if err := t.UnregisterName(); err != nil {
		tc.err <- err
		return
	}
	if err := t.UnregisterName(); err != gen.ErrNameUnknown {
		tc.err <- errIncorrect
		return
	}
	if err := t.RegisterName("newname"); err != nil {
		tc.err <- err
		return
	}
	if t.Name() != "newname" {
		tc.err <- errIncorrect
		return
	}
	tc.err <- nil
}

func (t *t1) TestPID(tc *testcase) {
	if reflect.DeepEqual(t.PID(), tc.output) == false {
		tc.err <- errIncorrect
		return
	}
	tc.err <- nil
}

func (t *t1) TestParentLeader(tc *testcase) {
	if reflect.DeepEqual(t.Parent(), tc.output) == false {
		tc.err <- errIncorrect
		return
	}
	if reflect.DeepEqual(t.Leader(), tc.output) == false {
		tc.err <- errIncorrect
		return
	}
	tc.err <- nil
}

func (t *t1) TestUptime(tc *testcase) {
	if t.Uptime() != 0 {
		tc.err <- errIncorrect
		return
	}
	tc.err <- nil
}

func (t *t1) TestState(tc *testcase) {
	if t.State() != gen.ProcessStateRunning {
		tc.err <- errIncorrect
		return
	}
	tc.err <- nil
}

func (t *t1) TestCompression(tc *testcase) {
	// enable/disable
	if t.Compression() != false {
		tc.err <- errIncorrect
		return
	}
	if err := t.SetCompression(true); err != nil {
		t.Log().Error("%s", err)
		tc.err <- err
		return
	}
	if t.Compression() != true {
		tc.err <- errIncorrect
		return
	}

	// level
	if t.CompressionLevel() != gen.DefaultCompressionLevel {
		tc.err <- errIncorrect
		return
	}
	if err := t.SetCompressionLevel(100); err != gen.ErrIncorrect {
		t.Log().Error("SetCompressionLevel (with invalid value): %s", err)
		tc.err <- errIncorrect
		return
	}
	if err := t.SetCompressionLevel(gen.CompressionBestSize); err != nil {
		t.Log().Error("SetCompressionLevel: %s", err)
		tc.err <- err
		return
	}
	if t.CompressionLevel() != gen.CompressionBestSize {
		t.Log().Error("CompressionLevel")
		tc.err <- errIncorrect
		return
	}
	// threshold
	if x := t.CompressionThreshold(); x != gen.DefaultCompressionThreshold {
		t.Log().Error("CompressionThreshold")
		tc.err <- errIncorrect
		return
	}

	if err := t.SetCompressionThreshold(1); err != gen.ErrIncorrect {
		t.Log().Error("SetCompressionThreshold: %s", err)
		tc.err <- errIncorrect
		return
	}
	if err := t.SetCompressionThreshold(gen.DefaultCompressionThreshold + 100); err != nil {
		t.Log().Error("SetCompressionThreshold (with invalid value): %s", err)
		tc.err <- errIncorrect
		return
	}
	if x := t.CompressionThreshold(); x != gen.DefaultCompressionThreshold+100 {
		tc.err <- errIncorrect
		return
	}
	tc.err <- nil
}

func (t *t1) TestSendPriority(tc *testcase) {
	if t.SendPriority() != gen.MessagePriorityNormal {
		tc.err <- errIncorrect
		return
	}

	if err := t.SetSendPriority(gen.MessagePriorityMax); err != nil {
		tc.err <- errIncorrect
		return
	}
	if x := t.SendPriority(); x != gen.MessagePriorityMax {
		tc.err <- errIncorrect
		return
	}
	if err := t.SetSendPriority(gen.MessagePriority(12345)); err != gen.ErrIncorrect {
		tc.err <- errIncorrect
		return
	}

	tc.err <- nil
}

func (t *t1) TestAliases(tc *testcase) {
	if len(t.Aliases()) != 0 {
		tc.err <- errIncorrect
		return
	}
	a1, err := t.CreateAlias()
	if err != nil {
		tc.err <- err
		return
	}

	a2, err := t.CreateAlias()
	if err != nil {
		tc.err <- err
		return
	}
	aliases := []gen.Alias{a1, a2}
	if reflect.DeepEqual(aliases, t.Aliases()) == false {
		tc.err <- errIncorrect
		return
	}
	if err := t.DeleteAlias(a1); err != nil {
		tc.err <- err
		return
	}
	aliases = []gen.Alias{a2}
	if reflect.DeepEqual(aliases, t.Aliases()) == false {
		tc.err <- errIncorrect
		return
	}
	tc.err <- nil
}
func (t *t1) TestEvents(tc *testcase) {
	e1 := gen.Atom("e1")
	e2 := gen.Atom("e2")
	if len(t.Events()) != 0 {
		tc.err <- errIncorrect
		return
	}
	opts := gen.EventOptions{
		Notify: true,
		Buffer: 10,
	}
	_, err := t.RegisterEvent(e1, opts)
	if err != nil {
		tc.err <- err
		return
	}

	_, err = t.RegisterEvent(e2, opts)
	if err != nil {
		tc.err <- err
		return
	}
	mevents := map[gen.Atom]bool{
		e1: true,
		e2: true,
	}
	ev := make(map[gen.Atom]bool)
	for _, e := range t.Events() {
		ev[e] = true
	}
	if reflect.DeepEqual(mevents, ev) == false {
		tc.err <- errIncorrect
		return
	}
	if err := t.UnregisterEvent(e1); err != nil {
		tc.err <- err
		return
	}
	events := []gen.Atom{e2}
	if reflect.DeepEqual(events, t.Events()) == false {
		tc.err <- errIncorrect
		return
	}
	tc.err <- nil
}

func (t *t1) TestSpawn(tc *testcase) {
	factory := func() gen.ProcessBehavior {
		x := struct {
			act.Actor
		}{}
		return &x
	}
	pid, err := t.Spawn(factory, gen.ProcessOptions{})
	if err != nil {
		tc.err <- err
		return
	}
	info, err := t.Node().ProcessInfo(pid)
	if err != nil {
		tc.err <- err
		return
	}
	if info.Parent != t.PID() {
		tc.err <- errIncorrect
		return
	}

	pid, err = t.SpawnRegister("reg", factory, gen.ProcessOptions{})
	if err != nil {
		tc.err <- err
		return
	}
	info, err = t.Node().ProcessInfo(pid)
	if err != nil {
		tc.err <- err
		return
	}
	if info.Parent != t.PID() {
		tc.err <- errIncorrect
		return
	}
	if info.Name != "reg" {
		tc.err <- errIncorrect
		return
	}

	tc.err <- nil
}

func TestT1ProcessBasic(t *testing.T) {
	nenv := map[gen.Env]any{
		gen.Env("A"): 1,
		gen.Env("B"): 1.23,
		gen.Env("C"): "d",
	}
	nopt := gen.NodeOptions{
		Env: nenv,
	}
	nopt.Log.DefaultLogger.Disable = true
	// nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("t1node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	penv := map[gen.Env]any{
		gen.Env("B"): 1.23,
		gen.Env("D"): "d",
	}
	popt := gen.ProcessOptions{
		Env: penv,
	}
	pid, err := node.SpawnRegister(gen.Atom("a"), factory_t1, popt)
	if err != nil {
		panic(err)
	}

	expenv := nenv
	for k, v := range penv {
		expenv[k] = v
	}

	t1cases = []*testcase{
		{"TestNodeInterface", nil, node, make(chan error)},
		{"TestEnv", nil, expenv, make(chan error)},
		{"TestName", nil, gen.Atom("a"), make(chan error)},
		{"TestPID", nil, pid, make(chan error)},
		{"TestParentLeader", nil, node.PID(), make(chan error)},
		{"TestUptime", nil, nil, make(chan error)},
		{"TestState", nil, nil, make(chan error)},
		{"TestCompression", nil, nil, make(chan error)},
		{"TestSendPriority", nil, nil, make(chan error)},
		{"TestAliases", nil, nil, make(chan error)},
		{"TestEvents", nil, nil, make(chan error)},
		{"TestSpawn", nil, nil, make(chan error)},
	}
	for _, tc := range t1cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(10); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
