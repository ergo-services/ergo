package gen

import (
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

var (
	WebHandlerStatusOK   WebHandlerStatus = nil
	WebHandlerStatusWait WebHandlerStatus = fmt.Errorf("wait")

	defaultIdleTimeout = 15
)

type WebHandlerStatus error

type WebHandlerBehavior interface {
	ServerBehavior

	// Mandatory callback
	HandleRequest(process *WebHandlerProcess, request WebMessageRequest) WebHandlerStatus

	// Optional callbacks
	HandleWebHandlerCall(process *WebHandlerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)
	HandleWebHandlerCast(process *WebHandlerProcess, message etf.Term) ServerStatus
	HandleWebHandlerInfo(process *WebHandlerProcess, message etf.Term) ServerStatus

	// internal methods
	initHandler(process Process, handler WebHandlerBehavior, options WebHandlerOptions) (http.Handler, error)
}

type WebHandler struct {
	Server

	parent   Process
	behavior WebHandlerBehavior
	options  WebHandlerOptions
	mutex    sync.Mutex
	pool     []handlerProcess
	counter  uint64
}

type handlerProcess struct {
	process  Process
	failures int32
}

type WebHandlerOptions struct {
	// Timeout for request
	Timeout int
	// NumHandlers defines how many handlers should be started in the pool. Default 1
	NumHandlers int
	// NumHandlers defines how big this pool could growth.
	MaxHandlers int
	// IdleTimeout defines how long the process in the pool can be alive with no requests. This option
	// affects the only extra processes
	IdleTimeout int
}

type WebHandlerProcess struct {
	ServerProcess
	behavior    WebHandlerBehavior
	lastRequest int64
}

type messageWebHandlerIdleCheck struct{}

type handlerArgs struct {
	idleTimeout int
}

//
//
//
func (wh *WebHandler) initHandler(parent Process, handler WebHandlerBehavior, options WebHandlerOptions) (http.Handler, error) {
	cpu := runtime.NumCPU()
	if options.MaxHandlers < 1 {
		options.MaxHandlers = cpu
	}
	if options.MaxHandlers < options.NumHandlers {
		return wh, fmt.Errorf("option NumHandlers(%d) can not be greater MaxHandlers(%d)",
			options.NumHandlers, options.MaxHandlers)
	}
	if options.NumHandlers < 1 {
		options.NumHandlers = 1
	}
	if options.MaxHandlers > cpu {
		return wh, fmt.Errorf("option MaxHandlers can not be greater runtime.NumCPU value (%d)", cpu)
	}
	if options.Timeout < 1 {
		options.Timeout = DefaultCallTimeout
	}

	if options.IdleTimeout < 1 {
		options.IdleTimeout = defaultIdleTimeout
	}

	wh.parent = parent
	wh.behavior = handler
	wh.options = options

	opts := ProcessOptions{
		Context: parent.Context(),
	}

	for i := 0; i < options.NumHandlers; i++ {
		p, err := parent.Spawn("", opts, handler)
		if err != nil {
			return wh, err
		}
		hp := handlerProcess{
			process: p,
		}
		wh.pool = append(wh.pool, hp)
	}
	return wh, nil
}

func (wh *WebHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	l := uint64(len(wh.pool))
	// make round robin using the counter value
	c := atomic.AddUint64(&wh.counter, 1)

	///
	/// MUTEX for the mr message to get rid of race condition - response writer
	// shouldn't be used once we exit from this callback
	///
	mr := WebMessageRequest{
		Request:  r,
		Response: w,
	}

	// attempts
	for a := uint64(0); a < l; a++ {
		i := (c + a) % l
		hp := wh.pool[i]
		_, err := hp.process.DirectWithTimeout(mr, wh.options.Timeout)
		switch err {
		case nil:
			return
		case lib.ErrProcessTerminated:
			continue
		case lib.ErrTimeout:
			fmt.Println("AAAAAAAAAAA")
			atomic.AddInt32(&mr.Canceled, 1)
			f := atomic.AddInt32(&hp.failures, 1)
			if f > 5 {
				hp.process.Kill()
			}
		case lib.ErrProcessBusy:
			continue
		default:
			lib.Warning("WebHandler %s return error: %s", hp.process.Self(), err)
			continue
		}
	}

	// all handlers are busy

	if wh.options.NumHandlers == 1 {
		// no process to handle this request
		w.WriteHeader(http.StatusServiceUnavailable) // 503
		return
	}

	// we have reached the limit
	if l+1 > uint64(wh.options.MaxHandlers) {
		w.WriteHeader(http.StatusServiceUnavailable) // 503
		return
	}

	opts := ProcessOptions{
		Context: wh.parent.Context(),
	}

	args := handlerArgs{
		idleTimeout: wh.options.IdleTimeout,
	}

	p, err := wh.parent.Spawn("", opts, wh.behavior, args)
	if err != nil {
		lib.Warning("can not extend WebHandler pool: %s", err)
	}
	hp := handlerProcess{
		process: p,
	}
	wh.pool = append(wh.pool, hp)
}

func (wh *WebHandler) Init(process *ServerProcess, args ...etf.Term) error {
	fmt.Println("Started new WebHandler process", process.Self())

	behavior, ok := process.Behavior().(WebHandlerBehavior)
	if !ok {
		return fmt.Errorf("Web: not a WebHandlerBehavior")
	}

	handlerProcess := &WebHandlerProcess{
		ServerProcess: *process,
		behavior:      behavior,
	}
	// do not inherit parent State
	handlerProcess.State = nil
	process.State = handlerProcess

	if args == nil {
		return nil
	}

	ha, ok := args[0].(handlerArgs)
	if ok == false {
		return fmt.Errorf("Web: wrong args for the WebHandlerBehavior")
	}
	if ha.idleTimeout > 0 {
		process.CastAfter(process.Self(), messageWebHandlerIdleCheck{}, 5*time.Second)
	}

	return nil
}

func (wh *WebHandler) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	whp := process.State.(*WebHandlerProcess)
	switch message.(type) {
	case messageWebHandlerIdleCheck:
		if time.Now().Unix()-whp.lastRequest > int64(wh.options.IdleTimeout) {
			return ServerStatusStop
		}
		process.CastAfter(process.Self(), messageWebHandlerIdleCheck{}, 5*time.Second)
	}
	return ServerStatusOK
}

func (wh *WebHandler) HandleDirect(process *ServerProcess, ref etf.Ref, message interface{}) (interface{}, DirectStatus) {
	whp := process.State.(*WebHandlerProcess)
	switch m := message.(type) {
	case WebMessageRequest:
		whp.lastRequest = time.Now().Unix()
		whp.behavior.HandleRequest(whp, m)
	}
	return nil, DirectStatusOK
}

// HandleWebHandlerCall
func (wh *WebHandler) HandleWebHandlerCall(process *WebHandlerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	lib.Warning("HandleWebHandlerCall: unhandled message (from %#v) %#v", from, message)
	return etf.Atom("ok"), ServerStatusOK
}

// HandleWebHandlerCast
func (wh *WebHandler) HandleWebHandlerCast(process *WebHandlerProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleWebHandlerCast: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleWebHandlerInfo
func (wh *WebHandler) HandleWebHandlerInfo(process *WebHandlerProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleWebHandlerInfo: unhandled message %#v", message)
	return ServerStatusOK
}

// we should disable SetTrapExit for the WebHandlerProcess by overriding it.
func (whp *WebHandlerProcess) SetTrapExit(trap bool) {
	lib.Warning("[%s] method 'SetTrapExit' is disabled for WebHandlerProcess", whp.Self())
}
