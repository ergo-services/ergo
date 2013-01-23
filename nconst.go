package node

// Distributed operations codes (http://www.erlang.org/doc/apps/erts/erl_dist_protocol.html)
const (
	LINK           = 1
	SEND           = 2
	EXIT           = 3
	UNLINK         = 4
	NODE_LINK      = 5
	REG_SEND       = 6
	GROUP_LEADER   = 7
	EXIT2          = 8
	SEND_TT        = 12
	EXIT_TT        = 13
	REG_SEND_TT    = 16
	EXIT2_TT       = 18
	MONITOR_P      = 19
	DEMONITOR_P    = 20
	MONITOR_P_EXIT = 21
)
