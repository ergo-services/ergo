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
type QuorumState int

var (
	RaftStatusOK   RaftStatus // nil
	RaftStatusStop RaftStatus = fmt.Errorf("stop")

	quorumState3  QuorumState = 3 // minimum quorum that could make leader election
	quorumState5  QuorumState = 5
	quorumState7  QuorumState = 7
	quorumState9  QuorumState = 9
	quorumState11 QuorumState = 11 // maximal quorum
)

type Raft struct {
	Server
}

type RaftProcess struct {
	ServerProcess
	options  RaftOptions
	behavior RaftBehavior

	quorum           Quorum
	quorumCandidates map[etf.Pid]bool
	quorumState      QuorumState
}

type Quorum struct {
	ID etf.Ref
	// the number of participants in quorum could be 3,5,7,9,11
	Participants []etf.Pid
	State        QuorumState
}

type RaftOptions struct {
	Peer ProcessID
	Data etf.Term
}

type messageRaft struct {
	Request etf.Atom
	Pid     etf.Pid
	Command interface{}
}

type messageRaftQuorumJoin struct{}
type messageRaftQuorumReply struct {
	Peers []etf.Pid
}

//
// RaftProcess quorum routines and APIs
//

func (rp *RaftProcess) Quorum() Quorum {
	q := Quorum{}
	for _, pid := range rp.quorum.Participants {
		q.Participants = append(q.Participants, pid)
	}
	q.ID = rp.quorum.ID
	q.State = rp.quorum.State

	return q
}

func (rp *RaftProcess) handleRaftRequest(m messageRaft) error {
	switch m.Request {
	case etf.Atom("$quorum_join"):
		fmt.Println("GOT QUO JOIN", rp.Name(), m.Pid)
		if _, exist := rp.quorumCandidates[m.Pid]; exist {
			return RaftStatusOK
		}
		peers := []etf.Pid{}
		for k, _ := range rp.quorumCandidates {
			peers = append(peers, k)
		}
		reply := etf.Tuple{
			etf.Atom("$quorum_join_reply"),
			rp.Self(),
			etf.Tuple{
				peers,
			},
		}
		rp.Cast(m.Pid, reply)
		rp.quorumCandidates[m.Pid] = true
		return RaftStatusOK

	case etf.Atom("$quorum_join_reply"):
		fmt.Println("GOT QUO JOIN REPL", rp.Name(), m.Pid)

		reply := &messageRaftQuorumReply{}
		if err := etf.TermIntoStruct(m.Command, &reply); err != nil {
			return ErrUnsupportedRequest
		}

		if _, exist := rp.quorumCandidates[m.Pid]; exist {
			return RaftStatusOK
		}
		rp.quorumCandidates[m.Pid] = true

		for _, peer := range reply.Peers {
			if _, exist := rp.quorumCandidates[peer]; exist {
				continue
			}
			join := etf.Tuple{
				etf.Atom("$quorum_join"),
				rp.Self(),
			}
			rp.Cast(peer, join)

		}
		return RaftStatusOK
	}

	return ErrUnsupportedRequest
}

//
// Server callbacks
//

func (r *Raft) Init(process *ServerProcess, args ...etf.Term) error {
	var options RaftOptions

	behavior, ok := process.Behavior().(RaftBehavior)
	if !ok {
		return fmt.Errorf("Raft: not a RaftBehavior")
	}

	raftProcess := &RaftProcess{
		ServerProcess:    *process,
		behavior:         behavior,
		quorumCandidates: make(map[etf.Pid]bool),
	}

	// do not inherit parent State
	raftProcess.State = nil
	options, err := behavior.InitRaft(raftProcess, args...)
	if err != nil {
		return err
	}

	raftProcess.options = options
	process.State = raftProcess

	noPeer := ProcessID{}
	if options.Peer == noPeer {
		return nil
	}

	join := etf.Tuple{
		etf.Atom("$quorum_join"),
		process.Self(),
	}
	process.Cast(options.Peer, join)

	//process.SetTrapExit(true)
	return nil
}

func (r *Raft) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	rp := process.State.(*RaftProcess)
	return rp.behavior.HandleRaftCall(rp, from, message)
}

func (r *Raft) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	var mRaft messageRaft

	rp := process.State.(*RaftProcess)

	if err := etf.TermIntoStruct(message, &mRaft); err != nil {
		return rp.behavior.HandleRaftInfo(rp, message)
	}

	status := rp.handleRaftRequest(mRaft)
	switch status {
	case nil, RaftStatusOK:
		return ServerStatusOK
	case RaftStatusStop:
		return ServerStatusStop
	case ErrUnsupportedRequest:
		return rp.behavior.HandleRaftInfo(rp, message)
	default:
		return ServerStatus(status)
	}

}

//
// default Raft callbacks
//

// HandleRaftCall
func (r *Raft) HandleRaftCall(process *RaftProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	lib.Warning("HandleRaftCall: unhandled message (from %#v) %#v", from, message)
	return etf.Atom("ok"), ServerStatusOK
}

// HandleRaftCast
func (r *Raft) HandleRaftCast(process *RaftProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleRaftCast: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleRaftInfo
func (r *Raft) HandleRaftInfo(process *RaftProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleRaftInfo: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleRaftDirect
func (r *Raft) HandleRaftDirect(process *RaftProcess, message interface{}) (interface{}, error) {
	return nil, ErrUnsupportedRequest
}
