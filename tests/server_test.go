package tests

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

type testServer struct {
	gen.Server
	res chan interface{}
}

func (tgs *testServer) Init(process *gen.ServerProcess, args ...etf.Term) error {
	tgs.res <- nil
	return nil
}
func (tgs *testServer) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	tgs.res <- message
	return gen.ServerStatusOK
}
func (tgs *testServer) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return message, gen.ServerStatusOK
}
func (tgs *testServer) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	tgs.res <- message
	return gen.ServerStatusOK
}

func (tgs *testServer) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case makeCall:
		return process.Call(m.to, m.message)
	case makeCast:
		return nil, process.Cast(m.to, m.message)

	}
	return nil, gen.ErrUnsupportedRequest
}
func (tgs *testServer) Terminate(process *gen.ServerProcess, reason string) {
	tgs.res <- reason
}

type testServerDirect struct {
	gen.Server
	err chan error
}

func (tgsd *testServerDirect) Init(process *gen.ServerProcess, args ...etf.Term) error {
	tgsd.err <- nil
	return nil
}
func (tgsd *testServerDirect) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {
	return message, nil
}

func TestServer(t *testing.T) {
	fmt.Printf("\n=== Test Server\n")
	fmt.Printf("Starting nodes: nodeGS1@localhost, nodeGS2@localhost: ")
	node1, _ := ergo.StartNode("nodeGS1@localhost", "cookies", node.Options{})
	node2, _ := ergo.StartNode("nodeGS2@localhost", "cookies", node.Options{})
	if node1 == nil || node2 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs1 := &testServer{
		res: make(chan interface{}, 2),
	}
	gs2 := &testServer{
		res: make(chan interface{}, 2),
	}
	gs3 := &testServer{
		res: make(chan interface{}, 2),
	}
	gsDirect := &testServerDirect{
		err: make(chan error, 2),
	}

	fmt.Printf("    wait for start of gs1 on %#v: ", node1.NodeName())
	node1gs1, _ := node1.Spawn("gs1", gen.ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.res, nil)

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.NodeName())
	node1gs2, _ := node1.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.res, nil)

	fmt.Printf("    wait for start of gs3 on %#v: ", node2.NodeName())
	node2gs3, _ := node2.Spawn("gs3", gen.ProcessOptions{}, gs3, nil)
	waitForResultWithValue(t, gs3.res, nil)

	fmt.Printf("    wait for start of gsDirect on %#v: ", node2.NodeName())
	node2gsDirect, _ := node2.Spawn("gsDirect", gen.ProcessOptions{}, gsDirect, nil)
	waitForResult(t, gsDirect.err)

	fmt.Println("Testing Server process:")

	fmt.Printf("    process.Send (by Pid) local (gs1) -> local (gs2) : ")
	node1gs1.Send(node1gs2.Self(), etf.Atom("hi"))
	waitForResultWithValue(t, gs2.res, etf.Atom("hi"))

	cast := makeCast{
		to:      node1gs2.Self(),
		message: etf.Atom("hi cast"),
	}
	node1gs1.Direct(cast)
	fmt.Printf("    process.Cast (by Pid) local (gs1) -> local (gs2) : ")
	waitForResultWithValue(t, gs2.res, etf.Atom("hi cast"))

	fmt.Printf("    process.Call (by Pid) local (gs1) -> local (gs2): ")
	v := etf.Atom("hi call")
	call := makeCall{
		to:      node1gs2.Self(),
		message: v,
	}
	if v1, err := node1gs1.Direct(call); err != nil {
		t.Fatal(err)
	} else {
		if v == v1 {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", v, v1)
			t.Fatal(e)
		}
	}

	fmt.Printf("    process.Send (by Name) local (gs1) -> local (gs2) : ")
	node1gs1.Send(etf.Atom("gs2"), etf.Atom("hi"))
	waitForResultWithValue(t, gs2.res, etf.Atom("hi"))

	cast = makeCast{
		to:      etf.Atom("gs2"),
		message: etf.Atom("hi cast"),
	}
	node1gs1.Direct(cast)
	fmt.Printf("    process.Cast (by Name) local (gs1) -> local (gs2) : ")
	waitForResultWithValue(t, gs2.res, etf.Atom("hi cast"))

	fmt.Printf("    process.Call (by Name) local (gs1) -> local (gs2): ")
	call = makeCall{
		to:      etf.Atom("gs2"),
		message: v,
	}
	if v1, err := node1gs1.Direct(call); err != nil {
		t.Fatal(err)
	} else {
		if v == v1 {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", v, v1)
			t.Fatal(e)
		}
	}
	alias, err := node1gs2.CreateAlias()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("    process.Send (by Alias) local (gs1) -> local (gs2) : ")
	node1gs1.Send(alias, etf.Atom("hi"))
	waitForResultWithValue(t, gs2.res, etf.Atom("hi"))

	cast = makeCast{
		to:      alias,
		message: etf.Atom("hi cast"),
	}
	node1gs1.Direct(cast)
	fmt.Printf("    process.Cast (by Alias) local (gs1) -> local (gs2) : ")
	waitForResultWithValue(t, gs2.res, etf.Atom("hi cast"))

	fmt.Printf("    process.Call (by Alias) local (gs1) -> local (gs2): ")
	call = makeCall{
		to:      alias,
		message: v,
	}
	if v1, err := node1gs1.Direct(call); err != nil {
		t.Fatal(err)
	} else {
		if v == v1 {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", v, v1)
			t.Fatal(e)
		}
	}

	fmt.Printf("    process.Send (by Pid) local (gs1) -> remote (gs3) : ")
	node1gs1.Send(node2gs3.Self(), etf.Atom("hi"))
	waitForResultWithValue(t, gs3.res, etf.Atom("hi"))

	cast = makeCast{
		to:      node2gs3.Self(),
		message: etf.Atom("hi cast"),
	}
	node1gs1.Direct(cast)
	fmt.Printf("    process.Cast (by Pid) local (gs1) -> remote (gs3) : ")
	waitForResultWithValue(t, gs3.res, etf.Atom("hi cast"))

	fmt.Printf("    process.Call (by Pid) local (gs1) -> remote (gs3): ")
	call = makeCall{
		to:      node2gs3.Self(),
		message: v,
	}
	if v1, err := node1gs1.Direct(call); err != nil {
		t.Fatal(err)
	} else {
		if v == v1 {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", v, v1)
			t.Fatal(e)
		}
	}

	fmt.Printf("    process.Send (by Name) local (gs1) -> remote (gs3) : ")
	processName := gen.ProcessID{Name: "gs3", Node: node2.NodeName()}
	node1gs1.Send(processName, etf.Atom("hi"))
	waitForResultWithValue(t, gs3.res, etf.Atom("hi"))

	cast = makeCast{
		to:      processName,
		message: etf.Atom("hi cast"),
	}
	node1gs1.Direct(cast)
	fmt.Printf("    process.Cast (by Name) local (gs1) -> remote (gs3) : ")
	waitForResultWithValue(t, gs3.res, etf.Atom("hi cast"))

	fmt.Printf("    process.Call (by Name) local (gs1) -> remote (gs3): ")
	call = makeCall{
		to:      processName,
		message: v,
	}
	if v1, err := node1gs1.Direct(call); err != nil {
		t.Fatal(err)
	} else {
		if v == v1 {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", v, v1)
			t.Fatal(e)
		}
	}

	fmt.Printf("    process.Send (by Alias) local (gs1) -> remote (gs3) : ")
	alias, err = node2gs3.CreateAlias()
	if err != nil {
		t.Fatal(err)
	}

	node1gs1.Send(alias, etf.Atom("hi"))
	waitForResultWithValue(t, gs3.res, etf.Atom("hi"))

	cast = makeCast{
		to:      alias,
		message: etf.Atom("hi cast"),
	}
	node1gs1.Direct(cast)
	fmt.Printf("    process.Cast (by Alias) local (gs1) -> remote (gs3) : ")
	waitForResultWithValue(t, gs3.res, etf.Atom("hi cast"))

	fmt.Printf("    process.Call (by Alias) local (gs1) -> remote (gs3): ")
	call = makeCall{
		to:      alias,
		message: v,
	}
	if v1, err := node1gs1.Direct(call); err != nil {
		t.Fatal(err)
	} else {
		if v == v1 {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", v, v1)
			t.Fatal(e)
		}
	}

	fmt.Printf("    process.Direct (without HandleDirect implementation): ")
	if _, err := node1gs1.Direct(nil); err == nil {
		t.Fatal("must be ErrUnsupportedRequest")
	} else {
		fmt.Println("OK")
	}
	fmt.Printf("    process.Direct (with HandleDirect implementation): ")
	if v1, err := node2gsDirect.Direct(v); err != nil {
		t.Fatal(err)
	} else {
		if v == v1 {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", v, v1)
			t.Fatal(e)
		}
	}

	fmt.Printf("    process.SetTrapExit(true) and call process.Exit() gs2: ")
	node1gs2.SetTrapExit(true)
	node1gs2.Exit("test trap")
	waitForResultWithValue(t, gs2.res, gen.MessageExit{Pid: node1gs2.Self(), Reason: "test trap"})
	fmt.Printf("    check process.IsAlive gs2 (must be alive): ")
	if !node1gs2.IsAlive() {
		t.Fatal("should be alive")
	}
	fmt.Println("OK")

	fmt.Printf("    process.SetTrapExit(false) and call process.Exit() gs2: ")
	node1gs2.SetTrapExit(false)
	node1gs2.Exit("test trap")
	waitForResultWithValue(t, gs2.res, "test trap")

	fmt.Printf("    check process.IsAlive gs2 (must be died): ")
	if node1gs2.IsAlive() {
		t.Fatal("shouldn't be alive")
	}
	fmt.Println("OK")

	fmt.Printf("Stopping nodes: %v, %v\n", node1.NodeName(), node2.NodeName())
	node1.Stop()
	node2.Stop()
}

type messageOrderGS struct {
	gen.Server
	n   int
	res chan interface{}
}

type testCase1 struct {
	n int
}

type testCase2 struct {
	n int
}
type testCase3 struct {
	n int
}

func (gs *messageOrderGS) Init(process *gen.ServerProcess, args ...etf.Term) error {
	gs.res <- nil
	return nil
}

func (gs *messageOrderGS) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	switch m := message.(type) {
	case testCase1:
		if gs.n+1 != m.n {
			panic(fmt.Sprintf("Disordered messages on %d (awaited: %d)", m.n, gs.n+1))
		}
		gs.n = m.n

		if gs.n == 100 {
			gs.res <- 1000
		}
		return gen.ServerStatusOK

	case testCase2:
		if gs.n != m.n {
			panic(fmt.Sprintf("Disordered messages on %d (awaited: %d)", m.n, gs.n+1))
		}
		gs.n = m.n - 1
		value, err := process.Call("gs3order", message)
		if err != nil {
			panic(err)
		}
		if value.(string) != "ok" {
			panic("wrong result")
		}

		if gs.n == 0 {
			gs.res <- 123
		}
		return gen.ServerStatusOK
	}

	return gen.ServerStatusStop
}

func (gs *messageOrderGS) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	switch message.(type) {
	case testCase2:
		return "ok", gen.ServerStatusOK
	case testCase3:
		return "ok", gen.ServerStatusOK
	}
	return nil, fmt.Errorf("incorrect call")
}

func (gs *messageOrderGS) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case testCase3:
		for i := 0; i < m.n; i++ {
			value, err := process.Call("gs3order", message)
			if err != nil {
				panic(err)
			}
			if value.(string) != "ok" {
				panic("wrong result")
			}
		}
		return nil, nil

	}
	return nil, fmt.Errorf("incorrect direct call")
}

type GSCallPanic struct {
	gen.Server
}

func (gs *GSCallPanic) Init(process *gen.ServerProcess, args ...etf.Term) error {
	return nil
}

func (gs *GSCallPanic) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	m := message.(string)
	if m == "panic" {
		panic("test")
	}

	return "ok", gen.ServerStatusOK
}

func (gs *GSCallPanic) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {

	pids, ok := message.([]etf.Pid)
	if !ok {
		return nil, fmt.Errorf("not a pid")
	}
	if _, err := process.CallWithTimeout(pids[0], "panic", 1); err == nil {
		return nil, fmt.Errorf("must be error here")
	}

	v, err := process.Call(pids[1], "test")
	if err != nil {
		return nil, err
	}
	if v.(string) != "ok" {
		return nil, fmt.Errorf("wrong result %#v", v)
	}

	return nil, nil
}

func TestServerCallServerWithPanic(t *testing.T) {
	fmt.Printf("\n=== Test Server. Making a Call to Server with panic (issue 86) \n")
	fmt.Printf("Starting node: nodeGSCallWithPanic1@localhost: ")
	node1, err1 := ergo.StartNode("nodeGSCallWithPanic1@localhost", "cookies", node.Options{})
	if err1 != nil {
		t.Fatal("can't start node", err1)
	} else {
		fmt.Println("OK")
	}
	fmt.Printf("Starting node: nodeGSCallWithPanic2@localhost: ")
	node2, err2 := ergo.StartNode("nodeGSCallWithPanic2@localhost", "cookies", node.Options{})
	if err2 != nil {
		t.Fatal("can't start node", err2)
	} else {
		fmt.Println("OK")
	}

	p1n1, err := node1.Spawn("", gen.ProcessOptions{}, &GSCallPanic{})
	if err != nil {
		t.Fatal(err)
	}
	p1n2, err := node2.Spawn("", gen.ProcessOptions{}, &GSCallPanic{})
	if err != nil {
		t.Fatal(err)
	}
	p2n2, err := node2.Spawn("", gen.ProcessOptions{}, &GSCallPanic{})
	if err != nil {
		t.Fatal(err)
	}

	pids := []etf.Pid{p1n2.Self(), p2n2.Self()}

	if _, err := p1n1.Direct(pids); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
}

func TestServerMessageOrder(t *testing.T) {
	fmt.Printf("\n=== Test Server message order\n")
	fmt.Printf("Starting node: nodeGS1MessageOrder@localhost: ")
	node1, _ := ergo.StartNode("nodeGS1MessageOrder@localhost", "cookies", node.Options{})
	if node1 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs1 := &messageOrderGS{
		res: make(chan interface{}, 2),
	}

	gs2 := &messageOrderGS{
		res: make(chan interface{}, 2),
	}

	gs3 := &messageOrderGS{
		res: make(chan interface{}, 2),
	}

	fmt.Printf("    wait for start of gs1order on %#v: ", node1.NodeName())
	node1gs1, err1 := node1.Spawn("gs1order", gen.ProcessOptions{}, gs1, nil)
	if err1 != nil {
		panic(err1)
	}
	waitForResultWithValue(t, gs1.res, nil)

	fmt.Printf("    wait for start of gs2order on %#v: ", node1.NodeName())
	node1gs2, err2 := node1.Spawn("gs2order", gen.ProcessOptions{}, gs2, nil)
	if err2 != nil {
		panic(err2)
	}
	waitForResultWithValue(t, gs2.res, nil)

	fmt.Printf("    wait for start of gs3order on %#v: ", node1.NodeName())
	node1gs3, err3 := node1.Spawn("gs3order", gen.ProcessOptions{}, gs3, nil)
	if err3 != nil {
		panic(err3)
	}
	waitForResultWithValue(t, gs3.res, nil)

	fmt.Printf("    sending 100 messages from gs1 to gs2. checking the order: ")
	for i := 1; i < 101; i++ {
		err := node1gs1.Send(node1gs2.Self(), testCase1{n: i})
		if err != nil {
			t.Fatal(err)
		}
	}
	waitForResultWithValue(t, gs2.res, 1000)
	fmt.Println("OK")

	fmt.Printf("    making Direct call with making a call from gs2 to gs3 1 time: ")
	_, err := node1gs2.Direct(testCase3{n: 1})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	fmt.Printf("    making Direct call with making a call from gs2 to gs3 100 times: ")
	_, err = node1gs2.Direct(testCase3{n: 100})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	gs2.n = 100

	fmt.Printf("    sending 100 messages from gs1 to gs2 with making a call from gs2 to gs3: ")
	for i := gs2.n; i > 0; i-- {
		err := node1gs1.Send(node1gs2.Self(), testCase2{n: i})
		if err != nil {
			t.Fatal(err)
		}
	}
	waitForResultWithValue(t, gs2.res, 123)
	fmt.Println("OK")
	node1gs3.Exit("normal")
	node1.Stop()
	node1.Wait()
}

type messageFloodSourceGS struct {
	gen.Server
	id  int
	res chan interface{}
}

type messageFlood struct {
	id int
	i  int
}

func (fl *messageFloodSourceGS) Init(process *gen.ServerProcess, args ...etf.Term) error {
	fl.res <- nil
	return nil
}

func (fl *messageFloodSourceGS) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	max := message.(int)

	for i := 1; i < max+1; i++ {
		if err := process.Send("gsdest", messageFlood{id: fl.id - 1, i: i}); err != nil {
			panic(fmt.Sprintf("err on making a send: %s", err))
		}
		if err := process.Cast("gsdest", messageFlood{id: fl.id - 1, i: i}); err != nil {
			panic(fmt.Sprintf("err on making a cast: %s", err))
		}
		if _, err := process.Call("gsdest", messageFlood{id: fl.id - 1, i: i}); err != nil {
			panic(fmt.Sprintf("err on making a call: %s", err))
		}

	}

	return gen.ServerStatusStop
}

type messageFloodDestGS struct {
	gen.Server
	max  int
	info [5]int
	cast [5]int
	call [5]int
	done int
	res  chan interface{}
}

func (fl *messageFloodDestGS) Init(process *gen.ServerProcess, args ...etf.Term) error {

	fl.res <- nil
	return nil
}

func (fl *messageFloodDestGS) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	switch m := message.(type) {
	case messageFlood:
		if fl.call[m.id]+1 != m.i {
			panic("wrong order")
		}
		fl.call[m.id] = m.i
		if fl.call[m.id] == fl.max {
			fl.done++
		}
		if fl.done != len(fl.info)*3 {
			return nil, gen.ServerStatusOK
		}
	default:
		return nil, gen.ServerStatusStop
	}

	fl.res <- nil
	return nil, gen.ServerStatusOK
}

func (fl *messageFloodDestGS) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	switch m := message.(type) {
	case messageFlood:
		if fl.cast[m.id]+1 != m.i {
			panic("wrong order")
		}
		fl.cast[m.id] = m.i
		if fl.cast[m.id] == fl.max {
			fl.done++
		}
		if fl.done != len(fl.info)*3 {
			return gen.ServerStatusOK
		}
	default:
		return gen.ServerStatusStop
	}

	fl.res <- nil
	return gen.ServerStatusOK
}

func (fl *messageFloodDestGS) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	switch m := message.(type) {
	case messageFlood:
		if fl.info[m.id]+1 != m.i {
			panic("wrong order")
		}
		fl.info[m.id] = m.i
		if fl.info[m.id] == fl.max {
			fl.done++
		}
		if fl.done != len(fl.info)*3 {
			return gen.ServerStatusOK
		}
	default:
		return gen.ServerStatusStop
	}

	fl.res <- nil
	return gen.ServerStatusOK
}

type testCaseFlood struct {
	id int
}

func TestServerMessageFlood(t *testing.T) {
	fmt.Printf("\n=== Test Server message flood \n")
	fmt.Printf("Starting node: nodeGS1MessageFlood@localhost: ")
	node1, _ := ergo.StartNode("nodeGS1MessageFlood@localhost", "cookies", node.Options{})
	if node1 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs1source := &messageFloodSourceGS{
		id:  1,
		res: make(chan interface{}, 2),
	}
	gs2source := &messageFloodSourceGS{
		id:  2,
		res: make(chan interface{}, 2),
	}
	gs3source := &messageFloodSourceGS{
		id:  3,
		res: make(chan interface{}, 2),
	}
	gs4source := &messageFloodSourceGS{
		id:  4,
		res: make(chan interface{}, 2),
	}
	gs5source := &messageFloodSourceGS{
		id:  5,
		res: make(chan interface{}, 2),
	}

	gsdest := &messageFloodDestGS{
		res: make(chan interface{}, 2),
	}
	fmt.Printf("    wait for start of gs1source on %#v: ", node1.NodeName())
	gs1sourceProcess, _ := node1.Spawn("gs1source", gen.ProcessOptions{}, gs1source, nil)
	waitForResultWithValue(t, gs1source.res, nil)

	fmt.Printf("    wait for start of gs2source on %#v: ", node1.NodeName())
	gs2sourceProcess, _ := node1.Spawn("gs2source", gen.ProcessOptions{}, gs2source, nil)
	waitForResultWithValue(t, gs2source.res, nil)

	fmt.Printf("    wait for start of gs3source on %#v: ", node1.NodeName())
	gs3sourceProcess, _ := node1.Spawn("gs3source", gen.ProcessOptions{}, gs3source, nil)
	waitForResultWithValue(t, gs3source.res, nil)

	fmt.Printf("    wait for start of gs4source on %#v: ", node1.NodeName())
	gs4sourceProcess, _ := node1.Spawn("gs4source", gen.ProcessOptions{}, gs4source, nil)
	waitForResultWithValue(t, gs4source.res, nil)

	fmt.Printf("    wait for start of gs5source on %#v: ", node1.NodeName())
	gs5sourceProcess, _ := node1.Spawn("gs5source", gen.ProcessOptions{}, gs5source, nil)
	waitForResultWithValue(t, gs5source.res, nil)

	fmt.Printf("    wait for start of gsdest on %#v: ", node1.NodeName())
	node1.Spawn("gsdest", gen.ProcessOptions{}, gsdest, nil)
	waitForResultWithValue(t, gsdest.res, nil)

	gsdest.max = 1000
	// start flood
	gs1sourceProcess.Send(gs1sourceProcess.Self(), gsdest.max)
	gs2sourceProcess.Send(gs2sourceProcess.Self(), gsdest.max)
	gs3sourceProcess.Send(gs3sourceProcess.Self(), gsdest.max)
	gs4sourceProcess.Send(gs4sourceProcess.Self(), gsdest.max)
	gs5sourceProcess.Send(gs5sourceProcess.Self(), gsdest.max)

	waitForResultWithValue(t, gsdest.res, nil)
}

func waitForResult(t *testing.T, w chan error) {
	select {
	case e := <-w:
		if e == nil {
			fmt.Println("OK")
			return
		}

		t.Fatal(e)

	case <-time.After(time.Second * time.Duration(1)):
		t.Fatal("result timeout")
	}
}

func waitForResultWithMultiValue(t *testing.T, w chan interface{}, values etf.List) {

	select {
	case v := <-w:
		found := false
		i := 0
		for {
			if reflect.DeepEqual(v, values[i]) {
				found = true
				values[i] = values[0]
				values = values[1:]
				if len(values) == 0 {
					return
				}
				// i dont care about stack growing since 'values'
				// usually short
				waitForResultWithMultiValue(t, w, values)
				break
			}
			i++
			if i+1 > len(values) {
				break
			}
		}

		if !found {
			e := fmt.Errorf("got unexpected value: %#v", v)
			t.Fatal(e)
		}

	case <-time.After(time.Second * time.Duration(2)):
		t.Fatal("result timeout")
	}
	fmt.Println("OK")
}

func waitForResultWithValue(t *testing.T, w chan interface{}, value interface{}) {
	select {
	case v := <-w:
		if reflect.DeepEqual(v, value) {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", value, v)
			t.Fatal(e)
		}

	case <-time.After(time.Second * time.Duration(2)):
		t.Fatal("result timeout")
	}
}

func waitForResultWithValueOrValue(t *testing.T, w chan interface{}, value1, value2 interface{}) {
	select {
	case v := <-w:
		if reflect.DeepEqual(v, value1) {
			fmt.Println("OK")
		} else {
			if reflect.DeepEqual(v, value2) {
				fmt.Println("OK")
			} else {
				e := fmt.Errorf("expected another value, but got: %#v", v)
				t.Fatal(e)
			}
		}

	case <-time.After(time.Second * time.Duration(2)):
		t.Fatal("result timeout")
	}
}

func waitForTimeout(t *testing.T, w chan interface{}) {
	select {
	case v := <-w:
		e := fmt.Errorf("got value we shouldn't receive: %#v", v)
		t.Fatal(e)

	case <-time.After(time.Millisecond * time.Duration(300)):
		return
	}
}
