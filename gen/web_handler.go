package gen

import (
	"fmt"
	"log"
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

	defaultRequestQueueLength = 10
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
	mutex    sync.RWMutex
	pool     []Process
	counter  uint64
}

type WebHandlerOptions struct {
	// Timeout for request
	RequestTimeout int
	// RequestQueueLength defines how many parallel requests can be directed to this process. Default value is 10.
	RequestQueueLength int
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
	counter     int64
}

type WebMessageRequest struct {
	Request  *http.Request
	Response http.ResponseWriter
}

type webMessageRequest struct {
	sync.Mutex
	WebMessageRequest
	requestState int // 0 - initial, 1 - canceled, 2 - handled
	ref          etf.Ref
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
	//	if options.MaxHandlers > cpu {
	//		return wh, fmt.Errorf("option MaxHandlers can not be greater runtime.NumCPU value (%d)", cpu)
	//	}
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

	for i := 0; i < options.NumHandlers; i++ {
		p := wh.startHandler()
		if p == nil {
			return nil, fmt.Errorf("can not initialize handlers")
		}
		wh.mutex.Lock()
		wh.pool = append(wh.pool, p)
		wh.mutex.Unlock()
	}
	return wh, nil
}

func (wh *WebHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	//w.WriteHeader(http.StatusOK)
	//return

	mr := &webMessageRequest{}
	mr.Request = r
	mr.Response = w
	mr.ref = wh.parent.MakeRef()

	timeout := wh.options.RequestTimeout
	if t := r.Header.Get("Request-Timeout"); t != "" {
		intT, err := strconv.Atoi(t)
		if err == nil && intT > 0 {
			timeout = intT
		}
	}

	wh.mutex.RLock()
	l := uint64(len(wh.pool))
	wh.mutex.RUnlock()
	// make round robin using the counter value
	c := atomic.AddUint64(&wh.counter, 1)

	// attempts
	for a := uint64(0); a < l; a++ {
		i := (c + a) % l
		p := wh.pool[i]

	respawned:
		if r.Context().Err() != nil {
			// canceled by the client
			return
		}
		_, err := p.DirectWithTimeout(mr, timeout)
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

			goto respawned

		case lib.ErrTimeout:
			mr.Lock()
			defer mr.Unlock()
			if mr.requestState == 2 {
				// timeout happened during the handling request
				return
			}
			log.Println(p.Self(), "request timed out")
			mr.requestState = 1 // canceled
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		case lib.ErrProcessBusy:
			//log.Println(mr.ref, "PROCESS BUSY", p.Self(), "i", i)
			continue
		default:
			lib.Warning("WebHandler %s return error: %s", p.Self(), err)
			continue
		}
	}

	// all handlers are busy

	wh.mutex.RLock()
	ll := len(wh.pool)
	wh.mutex.RUnlock()
	if ll+1 > wh.options.MaxHandlers {
		// we have reached the limit
		fmt.Println(mr.ref, "LIMIT REACHED", wh.options.MaxHandlers, "handlers", ll)
		w.WriteHeader(http.StatusServiceUnavailable) // 503
		return
	}

	p := wh.startHandler()
	if p == nil {
		w.WriteHeader(http.StatusInternalServerError) // 500
		return
	}

	wh.mutex.Lock()
	wh.pool = append(wh.pool, p)
	log.Println(mr.ref, "EXPAND HANDLERS POOL with", p.Self(), "total", len(wh.pool))
	wh.mutex.Unlock()

	if _, err := p.DirectWithTimeout(mr, timeout); err != nil {
		lib.Warning("WebHandler (expanded) %s return error: %s", p.Self(), err)
		mr.Lock()
		if mr.requestState == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			mr.requestState = 1
		}
		mr.Unlock()
	}
}

func (wh *WebHandler) startHandler() Process {
	opts := ProcessOptions{
		Context:       wh.parent.Context(),
		DirectboxSize: uint16(wh.options.RequestQueueLength),
	}

	p, err := wh.parent.Spawn("", opts, wh.behavior)
	if err != nil {
		lib.Warning("can not start WebHandler: %s", err)
		return nil
	}
	return p
}

func (wh *WebHandler) Init(process *ServerProcess, args ...etf.Term) error {
	fmt.Println(time.Now(), "Started new WebHandler process", process.Self(), "idle", wh.options.IdleTimeout)

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
		whp.counter++
		m.Lock()
		defer m.Unlock()
		if m.requestState != 0 || m.Request.Context().Err() != nil { // canceled
			log.Println(m.ref, "canceled", process.Self(), "handled", whp.counter)
			return nil, DirectStatusOK
		}
		m.requestState = 2 // handled
		if status := whp.behavior.HandleRequest(whp, m.WebMessageRequest); status == WebHandlerStatusWait {
			return nil, DirectStatusIgnore
		}
	}
	return nil, DirectStatusOK
}

func (wh *WebHandler) Terminate(process *ServerProcess, reason string) {
	whp := process.State.(*WebHandlerProcess)
	log.Println("TERMINATE", process.Self(), "handled", whp.counter)
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
