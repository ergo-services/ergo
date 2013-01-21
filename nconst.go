package node

import (
	"erlang/term"
)

const (
	LINK           term.Int = 1
	SEND           term.Int = 2
	EXIT           term.Int = 3
	UNLINK         term.Int = 4
	NODE_LINK      term.Int = 5
	REG_SEND       term.Int = 6
	GROUP_LEADER   term.Int = 7
	EXIT2          term.Int = 8
	SEND_TT        term.Int = 12
	EXIT_TT        term.Int = 13
	REG_SEND_TT    term.Int = 16
	EXIT2_TT       term.Int = 18
	MONITOR_P      term.Int = 19
	DEMONITOR_P    term.Int = 20
	MONITOR_P_EXIT term.Int = 21
)
