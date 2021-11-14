package node

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

var (
	ErrAppAlreadyLoaded     = fmt.Errorf("application is already loaded")
	ErrAppAlreadyStarted    = fmt.Errorf("application is already started")
	ErrAppUnknown           = fmt.Errorf("unknown application name")
	ErrAppIsNotRunning      = fmt.Errorf("application is not running")
	ErrNameUnknown          = fmt.Errorf("unknown name")
	ErrNameOwner            = fmt.Errorf("not an owner")
	ErrProcessBusy          = fmt.Errorf("process is busy")
	ErrProcessUnknown       = fmt.Errorf("unknown process")
	ErrProcessTerminated    = fmt.Errorf("process terminated")
	ErrBehaviorUnknown      = fmt.Errorf("unknown behavior")
	ErrBehaviorGroupUnknown = fmt.Errorf("unknown behavior group")
	ErrAliasUnknown         = fmt.Errorf("unknown alias")
	ErrAliasOwner           = fmt.Errorf("not an owner")
	ErrTaken                = fmt.Errorf("resource is taken")
	ErrTimeout              = fmt.Errorf("timed out")
	ErrFragmented           = fmt.Errorf("fragmented data")
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

type Node interface {
	gen.Registrar
	Network
	Name() string
	IsAlive() bool
	Uptime() int64
	Version() Version
	Spawn(name string, opts gen.ProcessOptions, object gen.ProcessBehavior, args ...etf.Term) (gen.Process, error)

	RegisterName(name string, pid etf.Pid) error
	UnregisterName(name string) error
	LoadedApplications() []gen.ApplicationInfo
	WhichApplications() []gen.ApplicationInfo
	ApplicationInfo(name string) (gen.ApplicationInfo, error)
	ApplicationLoad(app gen.ApplicationBehavior, args ...etf.Term) (string, error)
	ApplicationUnload(appName string) error
	ApplicationStart(appName string, args ...etf.Term) (gen.Process, error)
	ApplicationStartPermanent(appName string, args ...etf.Term) (gen.Process, error)
	ApplicationStartTransient(appName string, args ...etf.Term) (gen.Process, error)
	ApplicationStop(appName string) error
	ProvideRPC(module string, function string, fun gen.RPC) error
	RevokeRPC(module, function string) error

	Links(process etf.Pid) []etf.Pid
	Monitors(process etf.Pid) []etf.Pid
	MonitorsByName(process etf.Pid) []gen.ProcessID
	MonitoredBy(process etf.Pid) []etf.Pid

	Stop()
	Wait()
	WaitWithTimeout(d time.Duration) error
}

type Version struct {
	Release string
	Prefix  string
	OTP     int
}

type Network interface {
	AddStaticRoute(name string, port uint16) error
	AddStaticRouteExt(name string, port uint16, cookie string, tls bool) error
	RemoveStaticRoute(name string)
	Resolve(name string) (NetworkRoute, error)

	ProvideRemoteSpawn(name string, object gen.ProcessBehavior) error
	RevokeRemoteSpawn(name string) error
}

type NetworkRoute struct {
	Port   int
	Cookie string
	TLS    bool
}

// Options struct with bootstrapping options for CreateNode
type Options struct {
	Applications           []gen.ApplicationBehavior
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
