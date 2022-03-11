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

	quorumStateUnknown QuorumState = 0
	quorumState3       QuorumState = 3 // minimum quorum that could make leader election
	quorumState5       QuorumState = 5
	quorumState7       QuorumState = 7
	quorumState9       QuorumState = 9
	quorumState11      QuorumState = 11 // maximal quorum
)

type Raft struct {
	Server
}

type RaftProcess struct {
	ServerProcess
	options  RaftOptions
	behavior RaftBehavior

	quorumCandidates map[etf.Pid]etf.Ref
	quorumVotes      map[string]*quorum
	quorumState      QuorumState
}

type quorum struct {
	// the number of participants in quorum could be 3,5,7,9,11
	candidates []etf.Pid
	votes      map[etf.Pid]int // 1 - sent, 2 - recv, 3 - sent and recv
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
type messageRaftQuorumChange struct {
	ID         string
	Candidates []etf.Pid
}

//
// RaftProcess quorum routines and APIs
//

func (rp *RaftProcess) handleRaftRequest(m messageRaft) error {
	switch m.Request {
	case etf.Atom("$quorum_join"):
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
		mon := rp.MonitorProcess(m.Pid)
		rp.quorumCandidates[m.Pid] = mon
		fmt.Println(rp.Name(), "GOT QUO JOIN ", rp.quorumCandidates)
		return RaftStatusOK

	case etf.Atom("$quorum_join_reply"):

		reply := &messageRaftQuorumReply{}
		if err := etf.TermIntoStruct(m.Command, &reply); err != nil {
			return ErrUnsupportedRequest
		}

		if _, exist := rp.quorumCandidates[m.Pid]; exist {
			return RaftStatusOK
		}

		mon := rp.MonitorProcess(m.Pid)
		rp.quorumCandidates[m.Pid] = mon
		fmt.Println(rp.Name(), "GOT QUO JOIN REPL", rp.quorumCandidates)

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

		rp.quorumChange()
		return RaftStatusOK

	case etf.Atom("$quorum_change"):
		change := &messageRaftQuorumChange{}
		if err := etf.TermIntoStruct(m.Command, &change); err != nil {
			return ErrUnsupportedRequest
		}
		rp.quorumVote(m.Pid, change)
		return RaftStatusOK

	}

	return ErrUnsupportedRequest
}

func (rp *RaftProcess) quorumChange() {
	l := len(rp.quorumCandidates)
	switch {
	case l > 9:
		if rp.quorumState == quorumState11 {
			// do nothing
			return
		}
		l = 10 // to create quorum of 11 we need 10 candidates + itself.

	case l > 7:
		if rp.quorumState == quorumState9 {
			// do nothing
			return
		}
		l = 8 // quorum of 9 => 8 candidates + itself
	case l > 5:
		if rp.quorumState == quorumState7 {
			// do nothing
			return
		}
		l = 6 // quorum of 7 => 6 candidates + itself
	case l > 3:
		if rp.quorumState == quorumState5 {
			// do nothing
			return
		}
		l = 4 // quorum of 5 => 4 candidates + itself
	case l > 1:
		if rp.quorumState == quorumState3 {
			// do nothing
			return
		}
		l = 2 // quorum of 3 => 2 candidates + itself
	default:
		// not enougth candidates to create a quorum
		rp.quorumState = 0
		fmt.Println(rp.Name(), "QUO CHG. NOT ENO CAND", rp.quorumCandidates)
		return
	}

	fmt.Println(rp.Name(), "QUO CHG", l)
	candidates := make([]etf.Pid, l+1)
	candidates[0] = rp.Self()
	for c, _ := range rp.quorumCandidates {
		candidates[l] = c
		l--
		if l == 0 {
			break
		}
	}

	id := lib.RandomString(32)
	// send quorumChange to all candidates except itself
	quorumChange := etf.Tuple{
		etf.Atom("$quorum_change"),
		rp.Self(),
		etf.Tuple{
			id,
			candidates,
		},
	}
	quorum := &quorum{
		candidates: candidates,
		votes:      make(map[etf.Pid]int),
	}
	for _, pid := range candidates[1:] {
		fmt.Println(rp.Name(), "SEND QUO CHG to", pid)
		quorum.votes[pid] = 1
		rp.Cast(pid, quorumChange)
	}
	rp.quorumVotes[id] = quorum
	// TODO CastAfter(rp.Self(), messageRaftQuorumCleanVote{ID})
}

func (rp *RaftProcess) quorumVote(from etf.Pid, change *messageRaftQuorumChange) {
	fmt.Println(rp.Name(), "QUO VOTE", from, change)
	candidatesQuorumState := quorumStateUnknown
	switch QuorumState(len(change.Candidates)) {
	case quorumState3:
		candidatesQuorumState = quorumState3
	case quorumState5:
		candidatesQuorumState = quorumState5
	case quorumState7:
		candidatesQuorumState = quorumState7
	case quorumState9:
		candidatesQuorumState = quorumState9
	case quorumState11:
		candidatesQuorumState = quorumState11
	default:
		// wrong number of candidates
		// TODO exclude from candidates list and print warning
		return
	}

	q, exist := rp.quorumVotes[change.ID]
	if exist == false {
		q = &quorum{
			candidates: change.Candidates,
			votes:      make(map[etf.Pid]int),
		}
		rp.quorumVotes[change.ID] = q
		// TODO CastAfter(rp.Self(), messageRaftQuorumCleanVote{ID})
	}

	// mark as recv
	v := q.votes[from]
	v |= 2
	q.votes[from] = v

	candidatesMatch := true
	fmt.Println(rp.Name(), "AAA1", rp.quorumCandidates)
	candidatesVoted := true
	validFrom := false
	for _, pid := range q.candidates {
		if pid == rp.Self() {
			continue
		}
		if pid == from {
			validFrom = true
		}
		if _, exist := rp.quorumCandidates[pid]; exist == false {
			// TODO send join
			candidatesMatch = false
		}
		if v, _ := q.votes[pid]; v != 3 {
			candidatesVoted = false
		}
	}

	if validFrom == false {
		lib.Warning("%s got request from unknown quorum candidate: %#v", rp.Name(), from)
		return
	}

	fmt.Println(rp.Name(), "AAA2", candidatesMatch)
	if candidatesMatch == false {
		return
	}
	fmt.Println(rp.Name(), "AAA3", candidatesVoted)
	if candidatesVoted == true {
		// quorum formed
		fmt.Println(rp.Name(), "QUO FORMED ID:", change.ID)
		rp.quorumState = candidatesQuorumState
		return
	}

	candidatesVoted = true
	for _, pid := range q.candidates {
		if pid == rp.Self() {
			fmt.Println(rp.Name(), "AAA5 ign self", rp.Self())
			continue // do not send to itself
		}
		v, _ := q.votes[pid]

		// mark as sent
		q.votes[pid] = v | 1
		if v|1 != 3 {
			candidatesVoted = false
		}

		if v&1 > 0 {
			fmt.Println(rp.Name(), "AAA5 ign voted", pid)
			continue // already sent vote to this peer
		}

		fmt.Println(rp.Name(), "AAA5 send quo chg to", pid)
		// send quorum change request to the others
		quorumChange := etf.Tuple{
			etf.Atom("$quorum_change"),
			rp.Self(),
			etf.Tuple{
				change.ID,
				q.candidates,
			},
		}
		rp.Cast(pid, quorumChange)
	}

	if candidatesVoted == true {
		// quorum formed
		fmt.Println(rp.Name(), "QUO FORMED ID (after send):", change.ID)
		rp.quorumState = candidatesQuorumState
		return
	}
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
		quorumCandidates: make(map[etf.Pid]etf.Ref),
		quorumVotes:      make(map[string]*quorum),
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

// HandleCall
func (r *Raft) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	rp := process.State.(*RaftProcess)
	return rp.behavior.HandleRaftCall(rp, from, message)
}

// HandleCast
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

// HandleInfo
func (r *Raft) HandleInfo(process *ServerProcess, message etf.Term) ServerStatus {
	var status RaftStatus

	rp := process.State.(*RaftProcess)
	switch m := message.(type) {
	case MessageDown:
		mon, exist := rp.quorumCandidates[m.Pid]
		if m.Ref != mon {
			status = rp.behavior.HandleRaftInfo(rp, message)
			break
		}
		if exist == false {
			break
		}
		delete(rp.quorumCandidates, m.Pid)

		rp.quorumChange()

	default:
		status = rp.behavior.HandleRaftInfo(rp, message)
	}

	switch status {
	case nil, RaftStatusOK:
		return ServerStatusOK
	case RaftStatusStop:
		return ServerStatusStop
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
