package local

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/meta"
	"ergo.services/ergo/node"
)

var (
	t15cases []*testcase
)

func factory_t15() gen.ProcessBehavior {
	return &t15{}
}

type t15 struct {
	act.Actor

	testcase *testcase
}

func factory_t15web() gen.ProcessBehavior {
	return &t15web{}
}

type t15web struct {
	act.Web

	tc *testcase
}

func factory_t15handler() gen.ProcessBehavior {
	return &t15handler{}
}

type t15handler struct {
	act.Actor
}

func (t *t15handler) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case meta.MessageWebRequest:
		defer m.Done()
		t.Log().Info("got http request from %s", from)
		m.Response.WriteHeader(http.StatusAccepted)
	default:
		t.Log().Info("got unknown message from %s: %#v", from, message)
	}

	return nil
}

func (t *t15web) Init(args ...any) (act.WebOptions, error) {
	var options act.WebOptions

	options.Port = 12121
	options.Host = "localhost"

	mux := http.NewServeMux()

	handler1 := meta.CreateWebHandler(meta.WebHandlerOptions{}) // returns http.StatusNoContent
	if _, err := t.SpawnMeta(handler1, gen.MetaOptions{}); err != nil {
		return options, err
	}
	mux.Handle("/", handler1)

	opt := meta.WebHandlerOptions{
		Process: "handler", // must forward request to this process
	}
	handler2 := meta.CreateWebHandler(opt) // returns http.StatusAccepted
	if _, err := t.SpawnMeta(handler2, gen.MetaOptions{}); err != nil {
		return options, err
	}
	mux.Handle("/handler", handler2)

	handler3 := meta.CreateWebHandler(meta.WebHandlerOptions{})
	// do not start. check the case with no meta process
	mux.Handle("/nometaprocess", handler3) // returns http.StatusBadGateway

	options.Handler = mux
	return options, nil
}

func (t *t15web) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case meta.MessageWebRequest:
		defer m.Done()
		t.Log().Info("got http request from %s", from)
		m.Response.WriteHeader(http.StatusNoContent)
	default:
		t.Log().Info("got unknown message from %s: %#v", from, message)
	}

	return nil
}

func (t *t15) HandleMessage(from gen.PID, message any) error {
	if t.testcase == nil {
		t.testcase = message.(*testcase)
		message = initcase{}
	}
	// get method by name
	method := reflect.ValueOf(t).MethodByName(t.testcase.name)
	if method.IsValid() == false {
		t.testcase.err <- fmt.Errorf("unknown method %q", t.testcase.name)
		t.testcase = nil
		return nil
	}
	method.Call([]reflect.Value{reflect.ValueOf(message)})
	return nil
}

func (t *t15) TestBasic(input any) {
	defer func() {
		t.testcase = nil
	}()

	// start web-process
	webpid, err := t.Spawn(factory_t15web, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn web process: %s", err)
		t.testcase.err <- err
		return
	}

	// start handler-process
	handlerpid, err := t.SpawnRegister("handler", factory_t15handler, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn handler process: %s", err)
		t.testcase.err <- err
		return
	}

	// must be handler by web-process and return http.StatusNoContent
	url := "http://localhost:12121/"
	t.Log().Info("making request to %q. must be handled by %s (web)", url, webpid)
	r, err := http.Get(url)
	if err != nil {
		t.Log().Error("unable to make web request / : %s", err)
		t.testcase.err <- err
		return
	}

	if r.StatusCode != http.StatusNoContent {
		t.Log().Error("incorrect status code for /: %d (exp: %d)", r.StatusCode, http.StatusNoContent)
		t.testcase.err <- errIncorrect
		return
	}

	// must be handler by handler-process and return http.StatusAccepted
	url = "http://localhost:12121/handler"
	t.Log().Info("making request to %q. must be handled by %s (handler)", url, handlerpid)
	r, err = http.Get(url)
	if err != nil {
		t.Log().Error("unable to make web request / : %s", err)
		t.testcase.err <- err
		return
	}

	if r.StatusCode != http.StatusAccepted {
		t.Log().Error("incorrect status code for /: %d (exp: %d)", r.StatusCode, http.StatusAccepted)
		t.testcase.err <- errIncorrect
		return
	}

	// must be handler by meta-process itself and return http.StatusBadGateway
	url = "http://localhost:12121/nometaprocess"
	t.Log().Info("making request to %q. must be handled by meta-process itself", url)
	r, err = http.Get(url)
	if err != nil {
		t.Log().Error("unable to make web request / : %s", err)
		t.testcase.err <- err
		return
	}

	if r.StatusCode != http.StatusServiceUnavailable {
		t.Log().Error("incorrect status code for /: %d (exp: %d)", r.StatusCode, http.StatusServiceUnavailable)
		t.testcase.err <- errIncorrect
		return
	}

	// kill handlerpid and make request to the handler url. must be http.StatusBadGateway
	t.Node().Kill(handlerpid)

	url = "http://localhost:12121/handler"
	t.Log().Info("making request to %q. must be handled by meta-process (handler-process was killed)", url)
	r, err = http.Get(url)
	if err != nil {
		t.Log().Error("unable to make web request / : %s", err)
		t.testcase.err <- err
		return
	}

	if r.StatusCode != http.StatusBadGateway {
		t.Log().Error("incorrect status code for /: %d (exp: %d)", r.StatusCode, http.StatusBadGateway)
		t.testcase.err <- errIncorrect
		return
	}

	t.testcase.err <- nil
}

func TestT15Web(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	//nopt.Log.Level = gen.LogLevelTrace
	node, err := node.Start("t15Webnode@localhost", nopt, gen.Version{})
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t15, popt)
	if err != nil {
		panic(err)
	}

	t15cases = []*testcase{
		{"TestBasic", nil, nil, make(chan error)},
	}
	for _, tc := range t15cases {
		name := tc.name
		if tc.input != nil {
			name = fmt.Sprintf("%s:%s", tc.name, tc.input)
		}
		t.Run(name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(30); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
