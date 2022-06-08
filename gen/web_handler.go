package gen

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

var (
	WebHandlerStatusDone WebHandlerStatus = nil
	WebHandlerStatusWait WebHandlerStatus = fmt.Errorf("wait")

	defaultRequestQueueLength = 10

	webMessageRequestPool = &sync.Pool{
		New: func() interface{} {
			return &webMessageRequest{}
		},
	}
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
	HandleWebHandlerTerminate(process *WebHandlerProcess, reason string, count int64)

	// internal methods
	initHandler(process Process, handler WebHandlerBehavior, options WebHandlerOptions) (http.Handler, error)
}

type WebHandler struct {
	Server

	parent   Process
	behavior WebHandlerBehavior
	options  WebHandlerOptions
	pool     []*Process
	counter  uint64
}

type poolItem struct {
	process Process
}

type WebHandlerOptions struct {
	// Timeout for web-requests. The default timeout is 5 seconds. It can also be
	// overridden within HTTP requests using the header 'Request-Timeout'
	RequestTimeout int
	// RequestQueueLength defines how many parallel requests can be directed to this process. Default value is 10.
	RequestQueueLength int
	// NumHandlers defines how many handlers could be started in the main pool. Default 1
	NumHandlers int
	// IdleTimeout defines how long (in seconds) keep the started handler alive with no requests. Zero value makes handler not stop.
	IdleTimeout int
}

type WebHandlerProcess struct {
	ServerProcess
	behavior    WebHandlerBehavior
	lastRequest int64
	counter     int64
	idleTimeout int
	id          int
}

type WebMessageRequest struct {
	Ref      etf.Ref
	Request  *http.Request
	Response http.ResponseWriter
}

type webMessageRequest struct {
	sync.Mutex
	WebMessageRequest
	requestState int // 0 - initial, 1 - canceled, 2 - handled
}

type optsWebHandler struct {
	id          int
	idleTimeout int
}

type messageWebHandlerIdleCheck struct{}

func (wh *WebHandler) initHandler(parent Process, handler WebHandlerBehavior, options WebHandlerOptions) (http.Handler, error) {
	if options.NumHandlers < 1 {
		options.NumHandlers = 1
	}
	if options.RequestTimeout < 1 {
		options.RequestTimeout = DefaultCallTimeout
	}

	if options.IdleTimeout < 0 {
		options.IdleTimeout = 0
	}

	if options.RequestQueueLength < 1 {
		options.RequestQueueLength = defaultRequestQueueLength
	}

	wh.parent = parent
	wh.behavior = handler
	wh.options = options
	c := atomic.AddUint64(&wh.counter, 1)
	if c > 1 {
		return nil, fmt.Errorf("you can not use the same object more than once")
	}

	for i := 0; i < options.NumHandlers; i++ {
		p := wh.startHandler(i, options.IdleTimeout)
		if p == nil {
			return nil, fmt.Errorf("can not initialize handlers")
		}
		wh.pool = append(wh.pool, &p)
	}
	return wh, nil
}

func (wh *WebHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var p Process

	//w.WriteHeader(http.StatusOK)
	//return

	mr := webMessageRequestPool.Get().(*webMessageRequest)
	mr.Request = r
	mr.Response = w
	mr.requestState = 0

	timeout := wh.options.RequestTimeout
	if t := r.Header.Get("Request-Timeout"); t != "" {
		intT, err := strconv.Atoi(t)
		if err == nil && intT > 0 {
			timeout = intT
		}
	}

	l := uint64(wh.options.NumHandlers)
	// make round robin using the counter value
	c := atomic.AddUint64(&wh.counter, 1)

	// attempts
	for a := uint64(0); a < l; a++ {
		i := (c + a) % l

		p = *(*Process)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&wh.pool[i]))))

	respawned:
		if r.Context().Err() != nil {
			// canceled by the client
			return
		}
		_, err := p.DirectWithTimeout(mr, timeout)
		switch err {
		case nil:
			webMessageRequestPool.Put(mr)
			return

		case lib.ErrProcessTerminated:
			mr.Lock()
			if mr.requestState > 0 {
				mr.Unlock()
				return
			}
			mr.Unlock()
			p = wh.startHandler(int(i), wh.options.IdleTimeout)
			atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&wh.pool[i])), unsafe.Pointer(&p))
			goto respawned

		case lib.ErrTimeout:
			mr.Lock()
			if mr.requestState == 2 {
				// timeout happened during the handling request
				mr.Unlock()
				webMessageRequestPool.Put(mr)
				return
			}
			mr.requestState = 1 // canceled
			mr.Unlock()
			w.WriteHeader(http.StatusGatewayTimeout)
			return

		case lib.ErrProcessBusy:
			continue
		default:
			lib.Warning("WebHandler %s return error: %s", p.Self(), err)
			mr.Lock()
			if mr.requestState > 0 {
				mr.Unlock()
				return
			}
			mr.Unlock()

			w.WriteHeader(http.StatusInternalServerError) // 500
			return
		}
	}

	// all handlers are busy
	name := reflect.ValueOf(wh.behavior).Elem().Type().Name()
	lib.Warning("too many requests for %s", name)
	w.WriteHeader(http.StatusServiceUnavailable) // 503
	webMessageRequestPool.Put(mr)
}

func (wh *WebHandler) startHandler(id int, idleTimeout int) Process {
	opts := ProcessOptions{
		Context:       wh.parent.Context(),
		DirectboxSize: uint16(wh.options.RequestQueueLength),
	}

	optsHandler := optsWebHandler{id: id, idleTimeout: idleTimeout}
	p, err := wh.parent.Spawn("", opts, wh.behavior, optsHandler)
	if err != nil {
		lib.Warning("can not start WebHandler: %s", err)
		return nil
	}
	return p
}

func (wh *WebHandler) Init(process *ServerProcess, args ...etf.Term) error {
	behavior, ok := process.Behavior().(WebHandlerBehavior)
	if !ok {
		return fmt.Errorf("Web: not a WebHandlerBehavior")
	}
	handlerProcess := &WebHandlerProcess{
		ServerProcess: *process,
		behavior:      behavior,
	}
	if len(args) == 0 {
		return fmt.Errorf("Web: can not start with no args")
	}

	if a, ok := args[0].(optsWebHandler); ok {
		handlerProcess.idleTimeout = a.idleTimeout
		handlerProcess.id = a.id
	} else {
		return fmt.Errorf("Web: wrong args for the WebHandler")
	}

	// do not inherit parent State
	handlerProcess.State = nil
	process.State = handlerProcess

	if handlerProcess.idleTimeout > 0 {
		process.CastAfter(process.Self(), messageWebHandlerIdleCheck{}, 5*time.Second)
	}

	return nil
}

func (wh *WebHandler) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	whp := process.State.(*WebHandlerProcess)
	return whp.behavior.HandleWebHandlerCall(whp, from, message)
}

func (wh *WebHandler) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	whp := process.State.(*WebHandlerProcess)
	switch message.(type) {
	case messageWebHandlerIdleCheck:
		if time.Now().Unix()-whp.lastRequest > int64(whp.idleTimeout) {
			return ServerStatusStop
		}
		process.CastAfter(process.Self(), messageWebHandlerIdleCheck{}, 5*time.Second)

	default:
		return whp.behavior.HandleWebHandlerCast(whp, message)
	}
	return ServerStatusOK
}

func (wh *WebHandler) HandleInfo(process *ServerProcess, message etf.Term) ServerStatus {
	whp := process.State.(*WebHandlerProcess)
	return whp.behavior.HandleWebHandlerInfo(whp, message)
}

func (wh *WebHandler) HandleDirect(process *ServerProcess, ref etf.Ref, message interface{}) (interface{}, DirectStatus) {
	whp := process.State.(*WebHandlerProcess)
	switch m := message.(type) {
	case *webMessageRequest:
		whp.lastRequest = time.Now().Unix()
		whp.counter++
		m.Lock()
		defer m.Unlock()
		if m.requestState != 0 || m.Request.Context().Err() != nil { // canceled
			return nil, DirectStatusOK
		}
		m.requestState = 2 // handled
		m.Ref = ref
		status := whp.behavior.HandleRequest(whp, m.WebMessageRequest)
		switch status {
		case WebHandlerStatusDone:
			return nil, DirectStatusOK

		case WebHandlerStatusWait:
			return nil, DirectStatusIgnore
		default:
			return nil, status
		}
	}
	return nil, DirectStatusOK
}

func (wh *WebHandler) Terminate(process *ServerProcess, reason string) {
	whp := process.State.(*WebHandlerProcess)
	whp.behavior.HandleWebHandlerTerminate(whp, reason, whp.counter)
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
func (wh *WebHandler) HandleWebHandlerTerminate(process *WebHandlerProcess, reason string, count int64) {
	return
}

// we should disable SetTrapExit for the WebHandlerProcess by overriding it.
func (whp *WebHandlerProcess) SetTrapExit(trap bool) {
	lib.Warning("[%s] method 'SetTrapExit' is disabled for WebHandlerProcess", whp.Self())
}
