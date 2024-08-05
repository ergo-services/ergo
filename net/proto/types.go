package proto

import (
	"ergo.services/ergo/gen"
	"ergo.services/ergo/net/edf"
)

const (
	protoName    string = "ENP" // Ergo Network Protocol
	protoRelease string = "R1"  // (Rev.1)

	protoMagic   byte = 78
	protoVersion byte = 1

	// for messages sent with Send* methods
	protoMessagePID        byte = 101
	protoMessageName       byte = 102
	protoMessageNameCache  byte = 103
	protoMessageAlias      byte = 104
	protoMessageEvent      byte = 105
	protoMessageEventCache byte = 106
	protoMessageExit       byte = 107

	// for requests made with Call* methods
	protoRequestPID           byte = 121
	protoRequestName          byte = 122
	protoRequestNameCache     byte = 123
	protoRequestAlias         byte = 124
	protoMessageResponse      byte = 129
	protoMessageResponseError byte = 130

	// termination messages
	protoMessageTerminatePID        byte = 181
	protoMessageTerminateName       byte = 182
	protoMessageTerminateNameCache  byte = 183
	protoMessageTerminateAlias      byte = 184
	protoMessageTerminateEvent      byte = 185
	protoMessageTerminateEventCache byte = 186

	// any structured message (link/monitor/spawn/etc...)
	protoMessageAny byte = 199

	// order: compressed -> encrypted -> fragmented -> proxy
	protoMessageZ byte = 200 // compressed
	protoMessageE byte = 201 // encrypted
	protoMessageF byte = 202 // fragmented
	protoMessageP byte = 203 // proxy

	// TODO
	// protoFragmentSize int = 65000
)

//
// Link/Unlink
//

type MessageLinkPID struct {
	Source gen.PID
	Target gen.PID
	Ref    gen.Ref
}

type MessageUnlinkPID struct {
	Source gen.PID
	Target gen.PID
	Ref    gen.Ref
}

type MessageLinkProcessID struct {
	Source gen.PID
	Target gen.ProcessID
	Ref    gen.Ref
}

type MessageUnlinkProcessID struct {
	Source gen.PID
	Target gen.ProcessID
	Ref    gen.Ref
}

type MessageLinkAlias struct {
	Source gen.PID
	Target gen.Alias
	Ref    gen.Ref
}

type MessageUnlinkAlias struct {
	Source gen.PID
	Target gen.Alias
	Ref    gen.Ref
}

type MessageLinkEvent struct {
	Source gen.PID
	Target gen.Event
	Ref    gen.Ref
}

type MessageUnlinkEvent struct {
	Source gen.PID
	Target gen.Event
	Ref    gen.Ref
}

//
// Monitor/Demonitor
//

type MessageMonitorPID struct {
	Source gen.PID
	Target gen.PID
	Ref    gen.Ref
}

type MessageDemonitorPID struct {
	Source gen.PID
	Target gen.PID
	Ref    gen.Ref
}

type MessageMonitorProcessID struct {
	Source gen.PID
	Target gen.ProcessID
	Ref    gen.Ref
}

type MessageDemonitorProcessID struct {
	Source gen.PID
	Target gen.ProcessID
	Ref    gen.Ref
}

type MessageMonitorAlias struct {
	Source gen.PID
	Target gen.Alias
	Ref    gen.Ref
}

type MessageDemonitorAlias struct {
	Source gen.PID
	Target gen.Alias
	Ref    gen.Ref
}

type MessageMonitorEvent struct {
	Source gen.PID
	Target gen.Event
	Ref    gen.Ref
}

type MessageDemonitorEvent struct {
	Source gen.PID
	Target gen.Event
	Ref    gen.Ref
}

//
// remote spawn and application start
//

type MessageSpawn struct {
	Name    gen.Atom
	Options gen.ProcessOptionsExtra
	Ref     gen.Ref
}

type MessageApplicationStart struct {
	Name    gen.Atom
	Mode    gen.ApplicationMode
	Options gen.ApplicationOptionsExtra
	Ref     gen.Ref
}

// TODO
// for updating cache
//

type MessageUpdateCache struct {
	AtomCache   map[uint16]gen.Atom
	AtomMapping map[gen.Atom]gen.Atom
	RegCache    map[uint16]string
	ErrCache    map[uint16]error
	Ref         gen.Ref
}

//
// result message for "any" requests (link/monitor/spawn etc...),
//

type MessageResult struct {
	Error  error
	Result any
	Ref    gen.Ref
}

//
// message types must be registered
//

func init() {
	types := []any{
		MessageLinkPID{},
		MessageUnlinkPID{},
		MessageLinkProcessID{},
		MessageUnlinkProcessID{},
		MessageLinkAlias{},
		MessageUnlinkAlias{},
		MessageLinkEvent{},
		MessageUnlinkEvent{},
		MessageMonitorPID{},
		MessageDemonitorPID{},
		MessageMonitorProcessID{},
		MessageDemonitorProcessID{},
		MessageMonitorAlias{},
		MessageDemonitorAlias{},
		MessageMonitorEvent{},
		MessageDemonitorEvent{},
		MessageSpawn{},
		MessageApplicationStart{},
		MessageUpdateCache{},
		MessageResult{},
	}

	for _, t := range types {
		err := edf.RegisterTypeOf(t)
		if err == nil || err == gen.ErrTaken {
			continue
		}
		panic(err)
	}
}
