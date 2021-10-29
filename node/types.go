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
	ErrProcessIncarnation   = fmt.Errorf("Process ID belongs to the previous incarnation")
	ErrProcessTerminated    = fmt.Errorf("Process terminated")
	ErrMonitorUnknown       = fmt.Errorf("Unknown monitor reference")
	ErrSenderUnknown        = fmt.Errorf("Unknown sender")
	ErrBehaviorUnknown      = fmt.Errorf("Unknown behavior")
	ErrBehaviorGroupUnknown = fmt.Errorf("Unknown behavior group")
	ErrAliasUnknown         = fmt.Errorf("Unknown alias")
	ErrAliasOwner           = fmt.Errorf("Not an owner")
	ErrNoRoute              = fmt.Errorf("No route to node")
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

	EnvKeyVersion EnvKey = "ergo:Version"
	EnvKeyNode    EnvKey = "ergo:Node"
)

// Node
type Node interface {
	gen.Core
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

	AddStaticRoute(name string, port uint16) error
	AddStaticRouteExt(name string, port uint16, cookie string, tls bool) error
	RemoveStaticRoute(name string)
	Resolve(name string) (NetworkRoute, error)

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

// CoreRouter routes messages from/to remote node
type CoreRouter interface {

	// implemented by registrar

	// RouteSend routes message by Pid
	RouteSend(from etf.Pid, to etf.Pid, message etf.Term) error
	// RouteSendReg routes message by registered process name (gen.ProcessID)
	RouteSendReg(from etf.Pid, to gen.ProcessID, message etf.Term) error
	// RouteSendAlias routes message by process alias
	RouteSendAlias(from etf.Pid, to etf.Alias, message etf.Term) error

	// implemented by monitor

	// RouteLink makes linking of the given two processes
	RouteLink(pidA etf.Pid, pidB etf.Pid) error
	// RouteUnlink makes unlinking of the given two processes
	RouteUnlink(pidA etf.Pid, pidB etf.Pid) error
	// RouteExit routes MessageExit to the linked process
	RouteExit(to etf.Pid, terminated etf.Pid, reason string) error
	// RouteMonitorReg makes monitor to the given registered process name (gen.ProcessID)
	RouteMonitorReg(by etf.Pid, process gen.ProcessID, ref etf.Ref) error
	// RouteMonitor makes monitor to the given Pid
	RouteMonitor(by etf.Pid, process etf.Pid, ref etf.Ref) error
	RouteDemonitor(by etf.Pid, ref etf.Ref) error
	RouteMonitorExitReg(to etf.Pid, terminated gen.ProcessID, reason string, ref etf.Ref) error
	RouteMonitorExit(to etf.Pid, terminated etf.Pid, reason string, ref etf.Ref) error

	RouteSpawnRequest() error
	RouteSpawnReply() error

	RouteProxy() error
	RouteProxyReg() error

	ProcessByPid(pid etf.Pid) gen.Process
	ProcessByName(name string) gen.Process
	ProcessByAlias(alias etf.Alias) gen.Process

	GetConnection(nodename string) (*Connection, error)
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

// ProtoOptions
type ProtoOptions struct {
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
	// Applications application list that must be started
	Applications []gen.ApplicationBehavior
	// Env node environment
	Env map[gen.EnvKey]interface{}

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
	Handshake    Handshake
	Proto        Proto
	ProtoOptions ProtoOptions
}

// Connection
type Connection struct {
	ConnectionInterface
	conn net.Conn
}

// ConnectionInterface
type ConnectionInterface interface {
	Send(from gen.Process, to etf.Pid, message etf.Term) error
	SendReg(from gen.Process, to gen.ProcessID, message etf.Term) error
	SendAlias(from gen.Process, to etf.Alias, message etf.Term) error

	Link(local gen.Process, remote etf.Pid) error
	Unlink(local gen.Process, remote etf.Pid) error
	LinkExit(to etf.Pid, terminated etf.Pid, reason string) error

	Monitor(by etf.Pid, process etf.Pid, ref etf.Ref) error
	Demonitor(by etf.Pid, process etf.Pid, ref etf.Ref) error
	MonitorExit(to etf.Pid, terminated etf.Pid, reason string, ref etf.Ref) error

	MonitorReg(by etf.Pid, process gen.ProcessID, ref etf.Ref) error
	DemonitorReg(by etf.Pid, process gen.ProcessID, ref etf.Ref) error
	MonitorExitReg(to etf.Pid, terminated gen.Process, reason string, ref etf.Ref) error

	SpawnRequest()

	Proxy()
	ProxyReg()
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
	Init(options ProtoOptions, router Router) error
	Serve(conn net.Conn, connection Connection)
}
