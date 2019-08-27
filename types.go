package ergonode

import (
	"net"

	"github.com/halturin/ergonode/etf"
)

type systemProcesses struct {
	netKernel        *netKernel
	globalNameServer *globalNameServer
	rpc              *rpc
	observer         *observer
}

// Distributed operations codes (http://www.erlang.org/doc/apps/erts/erl_dist_protocol.html)
const (
	LINK                   = 1
	SEND                   = 2
	EXIT                   = 3
	UNLINK                 = 4
	NODE_LINK              = 5
	REG_SEND               = 6
	GROUP_LEADER           = 7
	EXIT2                  = 8
	SEND_TT                = 12
	EXIT_TT                = 13
	REG_SEND_TT            = 16
	EXIT2_TT               = 18
	MONITOR                = 19
	DEMONITOR              = 20
	MONITOR_EXIT           = 21
	SEND_SENDER            = 22
	SEND_SENDER_TT         = 23
	PAYLOAD_EXIT           = 24
	PAYLOAD_EXIT_TT        = 25
	PAYLOAD_EXIT2          = 26
	PAYLOAD_EXIT2_TT       = 27
	PAYLOAD_MONITOR_P_EXIT = 28
)

type peer struct {
	conn net.Conn
	send chan []etf.Term
}
