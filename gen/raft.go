package gen

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

var (
	ErrRaftState = fmt.Errorf("incorrect raft state")
)

type RaftBehavior interface {
	//
	// Mandatory callbacks
	//

	InitRaft(process *RaftProcess, arr ...etf.Term) (RaftOptions, error)

	//
	// Server's callbacks
	//

	// HandleRaftCall this callback is invoked on ServerProcess.Call. This method is optional
	// for the implementation
	HandleRaftCall(process *RaftProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)
	// HandleStageCast this callback is invoked on ServerProcess.Cast. This method is optional
	// for the implementation
	HandleRaftCast(process *RaftProcess, message etf.Term) ServerStatus
	// HandleStageInfo this callback is invoked on Process.Send. This method is optional
	// for the implementation
	HandleRaftInfo(process *RaftProcess, message etf.Term) ServerStatus
	// HandleRaftDirect this callback is invoked on Process.Direct. This method is optional
	// for the implementation
	HandleRaftDirect(process *RaftProcess, message interface{}) (interface{}, error)
}

type RaftStatus error
type quorumState int

var (
	RaftStatusOK   RaftStatus // nil
	RaftStatusStop RaftStatus = fmt.Errorf("stop")

	quorumState3  quorumState = 3
	quorumState5  quorumState = 5
	quorumState7  quorumState = 7
	quorumState9  quorumState = 9
	quorumState11 quorumState = 11
)

type Raft struct {
	Server

	// the number of nodes in quorum could be 3,5,7,9,11
	quorum           []string
	quorumCandidates []string
	quorumState      quorumState
}

type RaftProcess struct {
	ServerProcess
	options  RaftOptions
	behavior RaftBehavior
}

type RaftOptions struct {
	Peer string
	Data etf.Term
}

type messageQuorumChange struct{}

//
// default Raft callbacks
//

func (r *Raft) Init(process *ServerProcess, args ...etf.Term) error {
	var options RaftOptions

	behavior, ok := process.Behavior().(RaftBehavior)
	if !ok {
		return fmt.Errorf("Raft: not a RaftBehavior")
	}

	raftProcess := &RaftProcess{
		ServerProcess: *process,
		behavior:      behavior,
	}

	// do not inherit parent State
	raftProcess.State = nil
	options, err := behavior.InitRaft(raftProcess, args...)
	if err != nil {
		return err
	}

	raftProcess.options = options
	process.State = raftProcess

	if options.Peer != "" {
		process.Cast(process.Self(), messageQuorumChange{})
	}

	//process.SetTrapExit(true)
	return nil
}

func (r *Raft) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	rp := process.State.(*RaftProcess)
	return rp.behavior.HandleRaftCall(rp, from, message)
}

func (r *Raft) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	var status RaftStatus
	rp := process.State.(*RaftProcess)

	switch message.(type) {
	case messageQuorumChange:

	default:
		status = rp.behavior.HandleRaftCast(rp, message)
	}

	switch status {
	case RaftStatusOK:
		return ServerStatusOK
	case RaftStatusStop:
		return ServerStatusStop
	default:
		return ServerStatus(status)
	}

}

// HandleRaftCall
func (r *Raft) HandleRaftCall(process *RaftProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	lib.Warning("HandleRaftCall: unhandled message (from %#v) %#v\n", from, message)
	return etf.Atom("ok"), ServerStatusOK
}

// HandleRaftCast
func (r *Raft) HandleRaftCast(process *RaftProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleRaftCast: unhandled message %#v\n", message)
	return ServerStatusOK
}

// HandleRaftInfo
func (r *Raft) HandleRaftInfo(process *RaftProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleRaftInfo: unhandled message %#v\n", message)
	return ServerStatusOK
}

// HandleRaftDirect
func (r *Raft) HandleRaftDirect(process *RaftProcess, message interface{}) (interface{}, error) {
	return nil, ErrUnsupportedRequest
}
