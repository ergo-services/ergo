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
	ErrBehaviorUnknown      = fmt.Errorf("Unknown behavior")
	ErrBehaviorGroupUnknown = fmt.Errorf("Unknown behavior group")
	ErrAliasUnknown         = fmt.Errorf("Unknown alias")
	ErrAliasOwner           = fmt.Errorf("Not an owner")
	ErrTaken                = fmt.Errorf("Resource is taken")
	ErrTimeout              = fmt.Errorf("Timed out")
	ErrFragmented           = fmt.Errorf("Fragmented data")
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
	distProtoPROXY = 1001

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

	ProvideRemoteSpawn(name string, object gen.ProcessBehavior) error
	RevokeRemoteSpawn(name string) error
}

type NetworkHandler interface {
	HandleSend(from etf.Pid, to etf.Pid, message etf.Term)
	HandleSendReg(from etf.Pid, to gen.ProcessID, message etf.Term)
	HandleSendAlias(from etf.Pid, to etf.Alias, message etf.Term)

	HandleLink(remote etf.Pid, local etf.Pid)
	HandleUnlink(remote etf.Pid, local etf.Pid)
	HandleExit(terminated etf.Pid, reason string)

	HandleMonitorReg(remote etf.Pid, process gen.ProcessID, ref etf.Ref)
	HandleMonitor(remote etf.Pid, process etf.Pid, ref etf.Ref)
	HandleDemonitor(ref etf.Ref)
	HandleMonitorExitReg(process gen.ProcessID, reason string)
	HandleMonitorExit(process etf.Pid, reason string)

	HandleSpawnRequest()
	HandleSpawnReply()

	HandleProxyMessage()
}

// NetworkRoute
type NetworkRoute struct {
	Port   int
	Cookie string
	TLS    bool
}

type Connection struct {
	ConnectionHandler
	Name          string
	Peer          *Connection
	Conn          net.Conn
	EncodeOptions etf.EncodeOptions
	DecodeOptions etf.DecodeOptions
}

type ConnectionHandler interface {
	Send(gen.Process, to etf.Pid, message etf.Term) error
	SendReg(gen.Process, to gen.ProcessID, message etf.Term) error
	SendAlias(gen.Process, to etf.Alias, message etf.Term) error

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
}

// Options struct with bootstrapping options for CreateNode
type Options struct {
	Applications           []gen.ApplicationBehavior
	ListenRangeBegin       uint16
	ListenRangeEnd         uint16
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
	CustomHandshake        Handshake

	// ConnectionHandlers defines the number of readers/writers per connection. Default is the number of CPU.
	ConnectionHandlers int

	cookie   string
	creation uint32
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

// Handshake defines handshake interface
type Handshake interface {
	// Start initiates handshake process
	Start(ctx context.Context, conn net.Conn) (*Connection, error)
	// Accept accepts handshake process initiated by another side of this connection
	Accept(ctx context.Context, conn net.Conn) (*Connection, error)
}

type Proto interface {
	// HandleConnection
	HandleConnection(c *Connection)
}
