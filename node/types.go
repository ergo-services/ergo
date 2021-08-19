package node

import (
	"fmt"
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
)
