package node

import (
	"fmt"
	"sync"
	"time"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
)

var (
	ErrAppAlreadyLoaded   = fmt.Errorf("Application is already loaded")
	ErrAppAlreadyStarted  = fmt.Errorf("Application is already started")
	ErrAppUnknown         = fmt.Errorf("Unknown application name")
	ErrAppIsNotRunning    = fmt.Errorf("Application is not running")
	ErrProcessBusy        = fmt.Errorf("Process is busy")
	ErrProcessUnknown     = fmt.Errorf("Unknown process")
	ErrProcessTerminated  = fmt.Errorf("Process terminated")
	ErrAliasUnknown       = fmt.Errorf("Unknown alias")
	ErrAliasOwner         = fmt.Errorf("Not an owner")
	ErrTaken              = fmt.Errorf("Resource is taken")
	ErrUnsupportedRequest = fmt.Errorf("Unsupported request")
	ErrTimeout            = fmt.Errorf("Timed out")
	ErrFragmented         = fmt.Errorf("Fragmented data")
	ErrStop               = fmt.Errorf("stop")
)

// Distributed operations codes (http://www.erlang.org/doc/apps/erts/erl_dist_protocol.html)
const (
	distProtoLINK                   = 1
	distProtoSEND                   = 2
	distProtoEXIT                   = 3
	distProtoUNLINK                 = 4
	distProtoNODE_LINK              = 5
	distProtoREG_SEND               = 6
	distProtoGROUP_LEADER           = 7
	distProtoEXIT2                  = 8
	distProtoSEND_TT                = 12
	distProtoEXIT_TT                = 13
	distProtoREG_SEND_TT            = 16
	distProtoEXIT2_TT               = 18
	distProtoMONITOR                = 19
	distProtoDEMONITOR              = 20
	distProtoMONITOR_EXIT           = 21
	distProtoSEND_SENDER            = 22
	distProtoSEND_SENDER_TT         = 23
	distProtoPAYLOAD_EXIT           = 24
	distProtoPAYLOAD_EXIT_TT        = 25
	distProtoPAYLOAD_EXIT2          = 26
	distProtoPAYLOAD_EXIT2_TT       = 27
	distProtoPAYLOAD_MONITOR_P_EXIT = 28
	distProtoSPAWN_REQUEST          = 29
	distProtoSPAWN_REQUEST_TT       = 30
	distProtoSPAWN_REPLY            = 31
	distProtoSPAWN_REPLY_TT         = 32
	distProtoALIAS_SEND             = 33
	distProtoALIAS_SEND_TT          = 34
	distProtoUNLINK_ID              = 35
	distProtoUNLINK_ID_ACK          = 36

	defaultListenRangeBegin  uint16 = 15000
	defaultListenRangeEnd    uint16 = 65000
	defaultEPMDPort          uint16 = 4369
	defaultSendQueueLength   int    = 100
	defaultRecvQueueLength   int    = 100
	defaultFragmentationUnit        = 65000
	defaultHandshakeVersion         = 5
)

// Options struct with bootstrapping options for CreateNode
type Options struct {
	ListenRangeBegin       uint16
	ListenRangeEnd         uint16
	Hidden                 bool
	EPMDPort               uint16
	DisableEPMDServer      bool
	DisableEPMD            bool // use static routes only
	SendQueueLength        int
	RecvQueueLength        int
	FragmentationUnit      int
	DisableHeaderAtomCache bool
	TLSMode                TLSModeType
	TLScrtServer           string
	TLSkeyServer           string
	TLScrtClient           string
	TLSkeyClient           string
	// HandshakeVersion. Allowed values 5 or 6. Default version is 5
	HandshakeVersion int
	// ConnectionHandlers defines the number of readers/writers per connection. Default is the number of CPU.
	ConnectionHandlers int

	cookie   string
	creation uint32
}

// TLSmodeType should be one of TLSmodeDisabled (default), TLSmodeAuto or TLSmodeStrict
type TLSmodeType string

type Node interface {
	gen.Registrar
	Network
	IsAlive() bool
	Wait()
	WaitWithTimeout(d time.Duration) error
	Spawn(name string, opts gen.ProcessOptions, object gen.ProcessBehavior, args ...etf.Term) (gen.Process, error)
	Stop()
}

type Network interface {
	AddStaticRoute(name string, port uint16) error
	AddStaticRouteExt(name string, port uint16, cookie string, tls bool) error
	RemoveStaticRoute(name string)
	ProvideRemoteSpawn(name string, object gen.ProcessBehavior)
	RevokeRemoteSpawn(name string) bool

	connect(to etf.Atom) error
}

// TLSmodeType should be one of TLSmodeDisabled (default), TLSmodeAuto or TLSmodeStrict
type TLSModeType string

const (
	// TLSModeDisabled no TLS encryption
	TLSModeDisabled TLSModeType = ""
	// TLSModeAuto generate self-signed certificate
	TLSModeAuto TLSModeType = "auto"
	// TLSModeStrict with validation certificate
	TLSModeStrict TLSModeType = "strict"
)

type peer struct {
	name string
	send []chan []etf.Term
	i    int
	n    int

	mutex sync.Mutex
}

func (p *peer) GetChannel() chan []etf.Term {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	c := p.send[p.i]

	p.i++
	if p.i < p.n {
		return c
	}

	p.i = 0
	return c
}
