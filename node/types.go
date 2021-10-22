package node

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

var (
	ErrAppAlreadyLoaded     = fmt.Errorf("Application is already loaded")
	ErrAppAlreadyStarted    = fmt.Errorf("Application is already started")
	ErrAppUnknown           = fmt.Errorf("Unknown application name")
	ErrAppIsNotRunning      = fmt.Errorf("Application is not running")
	ErrNameUnknown          = fmt.Errorf("Unknown name")
	ErrNameOwner            = fmt.Errorf("Not an owner")
	ErrProcessBusy          = fmt.Errorf("Process is busy")
	ErrProcessUnknown       = fmt.Errorf("Unknown process")
	ErrProcessTerminated    = fmt.Errorf("Process terminated")
	ErrSenderUnknown        = fmt.Errorf("Unknown sender")
	ErrBehaviorUnknown      = fmt.Errorf("Unknown behavior")
	ErrBehaviorGroupUnknown = fmt.Errorf("Unknown behavior group")
	ErrAliasUnknown         = fmt.Errorf("Unknown alias")
	ErrAliasOwner           = fmt.Errorf("Not an owner")
	ErrTaken                = fmt.Errorf("Resource is taken")
	ErrTimeout              = fmt.Errorf("Timed out")
	ErrFragmented           = fmt.Errorf("Fragmented data")

	ErrProtoUnsupported = fmt.Errorf("Not supported")
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

	// ergo operations codes
	distProtoPROXY     = 1001
	distProtoREG_PROXY = 1002

	// node options
	defaultListenRangeBegin  uint16 = 15000
	defaultListenRangeEnd    uint16 = 65000
	defaultEPMDPort          uint16 = 4369
	defaultSendQueueLength   int    = 100
	defaultRecvQueueLength   int    = 100
	defaultFragmentationUnit        = 65000

	ContextKeyVersion ContextKey = "version"
)

// ContextKey
type ContextKey string

// Node
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
	ProvideRemoteSpawn(name string, object gen.ProcessBehavior) error
	RevokeRemoteSpawn(name string) error

	Links(process etf.Pid) []etf.Pid
	Monitors(process etf.Pid) []etf.Pid
	MonitorsByName(process etf.Pid) []gen.ProcessID
	MonitoredBy(process etf.Pid) []etf.Pid

	Stop()
	Wait()
	WaitWithTimeout(d time.Duration) error
}

// Version
type Version struct {
	Release string
	Prefix  string
	OTP     int
}

// Network
type Network interface {
	AddStaticRoute(name string, port uint16) error
	AddStaticRouteExt(name string, port uint16, cookie string, tls bool) error
	RemoveStaticRoute(name string)
	Resolve(name string) (NetworkRoute, error)
}

// Router routes messages from/to the remote node
type Router interface {
	RouteSend(from etf.Pid, to etf.Pid, message etf.Term) error
	RouteSendReg(from etf.Pid, to gen.ProcessID, message etf.Term) error
	RouteSendAlias(from etf.Pid, to etf.Alias, message etf.Term) error

	RouteLink(remote etf.Pid, local etf.Pid) error
	RouteUnlink(remote etf.Pid, local etf.Pid) error
	RouteExit(terminated etf.Pid, reason string) error

	RouteMonitorReg(remote etf.Pid, process gen.ProcessID, ref etf.Ref) error
	RouteMonitor(remote etf.Pid, process etf.Pid, ref etf.Ref) error
	RouteDemonitor(ref etf.Ref) error
	RouteMonitorExitReg(process gen.ProcessID, reason string) error
	RouteMonitorExit(process etf.Pid, reason string) error

	RouteSpawnRequest() error
	RouteSpawnReply() error

	RouteProxy() error
	RouteProxyReg() error
}

// NetworkRoute
type NetworkRoute struct {
	Port   int
	Cookie string
	TLS    bool
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

// ConnectionOptions
type ConnectionOptions struct {
	MaxMessageSize int64
	// NumHandlers defines the number of readers/writers per connection. Default is the number of CPU.
	NumHandlers            int
	SendQueueLength        int
	RecvQueueLength        int
	FragmentationUnit      int
	DisableHeaderAtomCache bool
	Compression            bool
	TLS                    bool
}

// Options defines bootstrapping options for the node
type Options struct {
	// application list that must be started
	Applications []gen.ApplicationBehavior

	// Creation. Default value: uint32(time.Now().Unix())
	Creation uint32

	// network options
	ListenRangeBegin  uint16
	ListenRangeEnd    uint16
	EPMDPort          uint16
	DisableEPMDServer bool
	DisableEPMD       bool // use static routes only

	// TLS settings
	TLSMode      TLSModeType
	TLScrtServer string
	TLSkeyServer string
	TLScrtClient string
	TLSkeyClient string

	// transport options
	ConnectionOptions ConnectionOptions
	Handshake         Handshake
	Proto             Proto
}

type Connection struct {
	// Name peer node name
	Name          string
	EncodeOptions etf.EncodeOptions
	DecodeOptions etf.DecodeOptions
	CustomOptions interface{}
}

// Handshake defines handshake interface
type Handshake interface {
	// Init initialize handshake. Invokes on a start node or if it used
	// with AddRouteStatic
	Init(nodename string, cookie string, options ConnectionOptions) error
	// Start initiates handshake process.
	Start(ctx context.Context, conn net.Conn) (*Connection, error)
	// Accept accepts handshake process initiated by another side of this connection
	Accept(ctx context.Context, conn net.Conn) (*Connection, error)
}

// Proto template struct for the custom Proto implementation
type Proto struct {
	ProtoInterface
}

// Proto defines proto interface for the custom Proto implementation
type ProtoInterface interface {
	Init(options ConnectionOptions, router Router) error
	Serve(conn net.Conn, connection Connection)

	Send(from gen.Process, to etf.Pid, message etf.Term) error
	SendReg(from gen.Process, to gen.ProcessID, message etf.Term) error
	SendAlias(from gen.Process, to etf.Alias, message etf.Term) error

	Link(local gen.Process, remote etf.Pid) error
	Unlink(local gen.Process, remote etf.Pid) error
	SendExit(local etf.Pid, remote etf.Pid) error

	Monitor(local gen.Process, remote etf.Pid, ref etf.Ref) error
	MonitorReg(local gen.Process, remote gen.ProcessID, ref etf.Ref) error
	Demonitor(ref etf.Ref) error
	SendMonitorExitReg(process gen.Process, ref etf.Ref, reason string) error
	SendMonitorExit(process etf.Pid, ref etf.Ref, reason string) error

	SpawnRequest()

	Proxy()
	ProxyReg()
}
