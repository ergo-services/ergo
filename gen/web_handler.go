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
	// Timeout for web-requests. The default timeout is 5 seconds. It can also be
	// overridden within HTTP requests using the header 'Request-Timeout'
	RequestTimeout int
	// RequestQueueLength defines how many parallel requests can be directed to this process. Default value is 10.
	RequestQueueLength int
	// NumHandlers defines how many handlers should keep running in the pool. Default 1
	NumHandlers int
	// MaxHandlers defines how large this pool could be. It grows and shrinks dynamically
	// within the range of NumHandlers and MaxHandlers. All dynamically started handlers
	// keep running, according to the IdleTimeout value.
	MaxHandlers int
	// IdleTimeout defines how long (in seconds) keep the dynamically started handler alive with no requests. Zero value makes handler not stop.
	IdleTimeout int
}

type WebHandlerProcess struct {
	ServerProcess
	behavior    WebHandlerBehavior
	lastRequest int64
	counter     int64
	idleTimeout int
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
	idleTimeout int
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
	wh.mutex.Lock()
	if wh.pool != nil {
		wh.mutex.Unlock()
		return nil, fmt.Errorf("you can not use the same object more than once")
	}

	wh.pool = make([]Process, options.MaxHandlers)
	for i := 0; i < options.NumHandlers; i++ {
		// the main pool has handlers with no idle timeout
		p := wh.startHandler(0)
		if p == nil {
			wh.mutex.Unlock()
			return nil, fmt.Errorf("can not initialize handlers")
		}
		wh.pool[i] = p
	}
	wh.mutex.Unlock()
	return wh, nil
}

func (wh *WebHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

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

	l := uint64(wh.options.MaxHandlers)
	// make round robin using the counter value
	c := atomic.AddUint64(&wh.counter, 1)

	// attempts
	for a := uint64(0); a < l; a++ {
		i := (c + a) % l

		wh.mutex.RLock()
		p := wh.pool[i]
		wh.mutex.RUnlock()

	respawned:
		if r.Context().Err() != nil {
			// canceled by the client
			return
		}
		err := lib.ErrProcessTerminated
		if p != nil {
			_, err = p.DirectWithTimeout(mr, timeout)
		}
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

			if wh.options.IdleTimeout > 0 && i > uint64(wh.options.NumHandlers) {
				// it was a dynamically started handler with enabled idle timeout, skip it and
				// choose another from the main pool
				l = uint64(wh.options.NumHandlers)
				a--
				continue
			}

			// do respawn it if this handler is a member of main pool
			pold := p
			p = wh.startHandler(wh.options.IdleTimeout)
			if p == nil {
				w.WriteHeader(http.StatusServiceUnavailable) // 503
				return
			}
			wh.mutex.Lock()
			// if its already replaced by the other goroutine just use the started handler only once
			// and do not put in the pool
			if pold != nil && wh.pool[i] == pold {
				fmt.Println(mr.Ref, "REPLACING terminated", i, pold.Self(), " with new", p.Self())
				wh.pool[i] = p
			}
			wh.mutex.Unlock()

			goto respawned

		case lib.ErrTimeout:
			mr.Lock()
			if mr.requestState == 2 {
				// timeout happened during the handling request
				mr.Unlock()
				webMessageRequestPool.Put(mr)
				return
			}
			log.Println(p.Self(), "request timed out")
			mr.requestState = 1 // canceled
			mr.Unlock()
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		case lib.ErrProcessBusy:
			//log.Println(mr.Ref, "PROCESS BUSY", p.Self(), "i", i)
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

	wh.mutex.RLock()
	ll := len(wh.pool)
	wh.mutex.RUnlock()
	if ll-1 > wh.options.MaxHandlers {
		// we have reached the limit
		lib.Warning("WebHandler: limit(%d) has been exceeded(%d)", wh.options.MaxHandlers, ll)
		w.WriteHeader(http.StatusServiceUnavailable) // 503
		webMessageRequestPool.Put(mr)
		return
	}

	p := wh.startHandler(wh.options.IdleTimeout)
	if p == nil {
		w.WriteHeader(http.StatusInternalServerError) // 500
		webMessageRequestPool.Put(mr)
		return
	}

	wh.mutex.Lock()
	wh.pool = append(wh.pool, p)
	log.Println(mr.Ref, "EXPAND HANDLERS POOL with", p.Self(), "total", len(wh.pool))
	wh.mutex.Unlock()

	_, err := p.DirectWithTimeout(mr, timeout)
	if err == nil {
		webMessageRequestPool.Put(mr)
		return
	}

	lib.Warning("WebHandler (expanded) %s return error: %s", p.Self(), err)
	mr.Lock()
	if mr.requestState == 0 {
		w.WriteHeader(http.StatusInternalServerError)
		mr.requestState = 1
	}
	mr.Unlock()
}

func (wh *WebHandler) startHandler(idleTimeout int) Process {
	opts := ProcessOptions{
		Context:       wh.parent.Context(),
		DirectboxSize: uint16(wh.options.RequestQueueLength),
	}

	optsHandler := optsWebHandler{idleTimeout: idleTimeout}
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
	} else {
		return fmt.Errorf("Web: wrong args for the WebHandler")
	}
	fmt.Println(time.Now(), "Started new WebHandler process", process.Self(), "idle", handlerProcess.idleTimeout)

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
			log.Println(m.Ref, "canceled", process.Self(), "handled", whp.counter)
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
