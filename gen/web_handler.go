package gen

import (
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

var (
	WebHandlerStatusOK   WebHandlerStatus = nil
	WebHandlerStatusWait WebHandlerStatus = fmt.Errorf("wait")
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
	initHandler(process Process, handler WebHandlerBehavior, options WebHandlerPoolOptions) (http.Handler, error)
}

type WebHandler struct {
	Server

	parent   Process
	behavior WebHandlerBehavior
	options  WebHandlerPoolOptions
	mutex    sync.Mutex
	pool     []Process
	counter  uint64
}

type WebHandlerPoolOptions struct {
	// Timeout for request
	Timeout int
	// NumHandlers defines how many handlers should be started in the pool. Default 0
	NumHandlers int
	// NumHandlers defines how big this pool could growth.
	MaxHandlers int
	// IdleTimout defines how long the process in the pool can be alive with no requests. This option
	// affects the only extra processes
	IdleTimout int
}

type WebHandlerProcess struct {
	ServerProcess
	behavior WebHandlerBehavior
}

//
//
//
func (wh *WebHandler) initHandler(parent Process, handler WebHandlerBehavior, options WebHandlerPoolOptions) (http.Handler, error) {
	cpu := runtime.NumCPU()
	if options.MaxHandlers == 0 {
		options.MaxHandlers = cpu
	}
	if options.MaxHandlers < options.NumHandlers {
		return wh, fmt.Errorf("option NumHandlers(%d) can not be greater MaxHandlers(%d)",
			options.NumHandlers, options.MaxHandlers)
	}
	if options.NumHandlers > 0 && options.NumHandlers < 3 {
		return wh, fmt.Errorf("option NumHandlers can not be less than 3")
	}
	if options.MaxHandlers > cpu {
		return wh, fmt.Errorf("option MaxHandlers can not be greater runtime.NumCPU value (%d)", cpu)
	}
	if options.Timeout < 1 {
		options.Timeout = DefaultCallTimeout
	}

	wh.parent = parent
	wh.behavior = handler
	wh.options = options

	opts := ProcessOptions{
		Context: parent.Context(),
	}

	p, err := parent.Spawn("", opts, handler)
	if err != nil {
		return wh, err
	}
	wh.pool = append(wh.pool, p)
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
		handler := wh.pool[i]
		_, err := handler.DirectWithTimeout(mr, wh.options.Timeout)
		// must be
		// if err == ErrProcessBusy {
		// 	continue
		// }
		// if err == ErrTimout {
		// 	handler.failures++
		//  if handler.failures > 5 {
		//   handler.Kill()
		//   start new process and put it in the poll at index i
		// }
		// }
		//
		if err == nil {
			return
		}
		fmt.Println("ERR", err)
		if err != nil {
			continue
		}
	}

	// all handlers are busy

	if wh.options.NumHandlers == 0 {
		// pool is disabled
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

	p, err := wh.parent.Spawn("", opts, wh.behavior)
	if err != nil {
		lib.Warning("can not extend WebHandler pool: %s", err)
	}
	wh.pool = append(wh.pool, p)
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

	return nil
}

func (wh *WebHandler) HandleDirect(process *ServerProcess, ref etf.Ref, message interface{}) (interface{}, DirectStatus) {
	webp := process.State.(*WebHandlerProcess)
	switch m := message.(type) {
	case WebMessageRequest:
		webp.behavior.HandleRequest(webp, m)
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
