package gen

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

var (
	WebHandlerStatusOK   WebHandlerStatus = nil
	WebHandlerStatusWait WebHandlerStatus = fmt.Errorf("wait")

	defaultRequestsQueueLength = 10
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
	pool     []Process
	counter  uint64
}

type WebHandlerOptions struct {
	// Timeout for request
	Timeout int
	// RequestsQueueLength defines how many parallel requests can be directed to this process. Default value is 10.
	RequestsQueueLength int
	// NumHandlers defines how many handlers should be started in the pool. Default 1
	NumHandlers int
	// NumHandlers defines how big this pool could growth.
	MaxHandlers int
	// IdleTimeout defines how long (in seconds) keep this process alive with no requests. Zero value disables this timeout.
	IdleTimeout int
}

type WebHandlerProcess struct {
	ServerProcess
	behavior    WebHandlerBehavior
	lastRequest int64
}

type webMessageRequest struct {
	WebMessageRequest
	canceled bool
	ref      etf.Ref
}
type WebMessageRequest struct {
	Request  *http.Request
	Response http.ResponseWriter
}

type messageWebHandlerIdleCheck struct{}

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

	if options.IdleTimeout < 0 {
		options.IdleTimeout = 0
	}

	if options.RequestsQueueLength < 1 {
		options.RequestsQueueLength = defaultRequestsQueueLength
	}

	wh.parent = parent
	wh.behavior = handler
	wh.options = options

	for i := 0; i < options.NumHandlers; i++ {
		p := wh.startHandler()
		if p == nil {
			return nil, fmt.Errorf("can not initialize handlers")
		}
		wh.pool = append(wh.pool, p)
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
	mr := webMessageRequest{}
	mr.Request = r
	mr.Response = w
	mr.ref = wh.parent.MakeRef()

	timeout := wh.options.Timeout
	if t := r.Header.Get("Request-Timeout"); t != "" {
		intT, err := strconv.Atoi(t)
		if err == nil && intT > 0 {
			timeout = intT
		}
	}

	// attempts
	for a := uint64(0); a < l; a++ {
		i := (c + a) % l
		p := wh.pool[i]
		_, err := p.DirectWithTimeout(&mr, timeout)
		switch err {
		case nil:
			return

		case lib.ErrProcessTerminated:
			p1 := p
			p = wh.startHandler()
			if p == nil {
				w.WriteHeader(http.StatusServiceUnavailable) // 503
				return
			}
			fmt.Println(mr.ref, "REPLACING terminated", i, p1.Self(), " with new", p.Self())
			wh.pool[i] = p
			_, err := p.DirectWithTimeout(&mr, timeout)
			if err != nil {
				lib.Warning("WebHandler %s return error: %s", p.Self(), err)
				w.WriteHeader(http.StatusInternalServerError)
			}
			return

		case lib.ErrTimeout:
			mr.canceled = true
			//w.WriteHeader(http.StatusRequestTimeout)
			return
		case lib.ErrProcessBusy:
			fmt.Println(mr.ref, "PROCESS BUSY", p.Self(), "i", i)
			continue
		default:
			lib.Warning("WebHandler %s return error: %s", p.Self(), err)
			continue
		}
	}

	// all handlers are busy

	if l+1 > uint64(wh.options.MaxHandlers) {
		// we have reached the limit
		fmt.Println(mr.ref, "LIMIT REACHED")
		w.WriteHeader(http.StatusServiceUnavailable) // 503
		return
	}

	p := wh.startHandler()
	if p == nil {
		w.WriteHeader(http.StatusServiceUnavailable) // 503
	}
	fmt.Println(mr.ref, "EXPAND HANDLERS POOL with", p.Self())
	wh.pool = append(wh.pool, p)
	_, err := p.DirectWithTimeout(&mr, timeout)
	mr.canceled = true
	if err != nil {
		lib.Warning("WebHandler %s return error: %s", p.Self(), err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (wh *WebHandler) startHandler() Process {
	opts := ProcessOptions{
		Context:       wh.parent.Context(),
		DirectboxSize: uint16(wh.options.RequestsQueueLength),
	}

	p, err := wh.parent.Spawn("", opts, wh.behavior)
	if err != nil {
		lib.Warning("can not start WebHandler: %s", err)
		return nil
	}
	return p
}

func (wh *WebHandler) Init(process *ServerProcess, args ...etf.Term) error {
	fmt.Println(time.Now(), "Started new WebHandler process", process.Self())

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

	if ok == false {
		return fmt.Errorf("Web: wrong args for the WebHandlerBehavior")
	}
	if wh.options.IdleTimeout > 0 {
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
	case *webMessageRequest:
		whp.lastRequest = time.Now().Unix()
		if m.canceled {
			fmt.Println(m.ref, "canceled", process.Self())
			return nil, DirectStatusOK
		}
		whp.behavior.HandleRequest(whp, m.WebMessageRequest)
	}
	return nil, DirectStatusOK
}

func (wh *WebHandler) Terminate(process *ServerProcess, reason string) {
	fmt.Println(time.Now(), "STOPPING", process.Self())
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
