package gen

import (
	"fmt"
	"math/rand"
	"time"

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
	// Optional callbacks
	//

	// HandleQuorumChange
	HandleQuorumChange(process *RaftProcess, qs RaftQuorumState) RaftStatus

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
type RaftQuorumState int

var (
	RaftStatusOK   RaftStatus // nil
	RaftStatusStop RaftStatus = fmt.Errorf("stop")

	RaftQuorumStateWait    RaftQuorumState = -1 // quorum has been built without this candidate
	RaftQuorumStateUnknown RaftQuorumState = 0
	RaftQuorumState3       RaftQuorumState = 3 // minimum quorum that could make leader election
	RaftQuorumState5       RaftQuorumState = 5
	RaftQuorumState7       RaftQuorumState = 7
	RaftQuorumState9       RaftQuorumState = 9
	RaftQuorumState11      RaftQuorumState = 11 // maximal quorum

	cleanVoteTimeout = 300 * time.Millisecond
)

type Raft struct {
	Server
}

type RaftProcess struct {
	ServerProcess
	options  RaftOptions
	behavior RaftBehavior

	quorum           Quorum
	quorumCandidates map[etf.Pid]etf.Ref
	quorumVotes      map[string]*quorum
	quorumState      RaftQuorumState
}

type Quorum struct {
	ID    string
	Peers []etf.Pid
}
type quorum struct {
	state RaftQuorumState
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
type messageRaftQuorumChangeDefer struct{}
type messageRaftQuorumLeave struct {
	ID string
}
type messageRaftQuorumCleanVote struct {
	id string
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
		after := time.Duration(50+rand.Intn(450)) * time.Millisecond
		rp.CastAfter(rp.Self(), messageRaftQuorumChangeDefer{}, after)
		return RaftStatusOK

	case etf.Atom("$quorum_change"):
		change := &messageRaftQuorumChange{}
		if err := etf.TermIntoStruct(m.Command, &change); err != nil {
			return ErrUnsupportedRequest
		}

		return rp.quorumVote(m.Pid, change)

	case etf.Atom("$quorum_leave"):
		leave := &messageRaftQuorumLeave{}
		if err := etf.TermIntoStruct(m.Command, &leave); err != nil {
			return ErrUnsupportedRequest
		}

		if leave.ID != rp.quorum.ID {
			// this process is not belong this quorum
			return RaftStatusOK
		}

		fmt.Println(rp.Name(), "LEAV QUO", rp.quorum.ID, m.Pid)
		rp.quorumState = RaftQuorumStateUnknown
		status := rp.behavior.HandleQuorumChange(rp, rp.quorumState)

		if len(rp.quorumVotes) > 0 {
			// voting is in progress
			return status
		}

		after := time.Duration(50+rand.Intn(450)) * time.Millisecond
		rp.CastAfter(rp.Self(), messageRaftQuorumChangeDefer{}, after)
		return status
	}

	return ErrUnsupportedRequest
}

func (rp *RaftProcess) quorumChange() RaftStatus {
	l := len(rp.quorumCandidates)
	candidateRaftQuorumState := RaftQuorumStateUnknown
	switch {
	case l > 9:
		if rp.quorumState == RaftQuorumState11 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState11
		l = 10 // to create quorum of 11 we need 10 candidates + itself.

	case l > 7:
		if rp.quorumState == RaftQuorumState9 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState9
		l = 8 // quorum of 9 => 8 candidates + itself
	case l > 5:
		if rp.quorumState == RaftQuorumState7 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState7
		l = 6 // quorum of 7 => 6 candidates + itself
	case l > 3:
		if rp.quorumState == RaftQuorumState5 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState5
		l = 4 // quorum of 5 => 4 candidates + itself
	case l > 1:
		if rp.quorumState == RaftQuorumState3 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState3
		l = 2 // quorum of 3 => 2 candidates + itself
	default:
		// not enougth candidates to create a quorum
		if rp.quorumState != RaftQuorumStateUnknown {
			rp.quorumState = RaftQuorumStateUnknown
			return rp.behavior.HandleQuorumChange(rp, RaftQuorumStateUnknown)
		}
		fmt.Println(rp.Name(), "QUO CHG. NOT ENO CAND", rp.quorumCandidates)
		return RaftStatusOK
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
		state:      candidateRaftQuorumState,
		candidates: candidates,
		votes:      make(map[etf.Pid]int),
	}
	for _, pid := range candidates[1:] {
		fmt.Println(rp.Name(), "SEND QUO CHG to", pid, id)
		quorum.votes[pid] = 1
		rp.Cast(pid, quorumChange)
	}
	rp.quorumVotes[id] = quorum
	rp.CastAfter(rp.Self(), messageRaftQuorumCleanVote{id: id}, cleanVoteTimeout)
	return RaftStatusOK
}

func (rp *RaftProcess) quorumVote(from etf.Pid, change *messageRaftQuorumChange) RaftStatus {
	fmt.Println(rp.Name(), "QUO VOTE", from, change)
	if rp.quorumState != RaftQuorumStateUnknown && len(change.Candidates) < int(rp.quorumState)+1 {
		// do not vote if requested quorum is less than existing one
		fmt.Println("SKIP VOTE", rp.Name())
		return RaftStatusOK
	}
	candidatesRaftQuorumState := RaftQuorumStateUnknown
	switch len(change.Candidates) {
	case 3:
		candidatesRaftQuorumState = RaftQuorumState3
	case 5:
		candidatesRaftQuorumState = RaftQuorumState5
	case 7:
		candidatesRaftQuorumState = RaftQuorumState7
	case 9:
		candidatesRaftQuorumState = RaftQuorumState9
	case 11:
		candidatesRaftQuorumState = RaftQuorumState11
	default:
		lib.Warning("[%s] wrong number of candidates in the request. removing %s from quorum candidates list", rp.Self(), from)
		delete(rp.quorumCandidates, from)
		return RaftStatusOK
	}

	// check for already voted quorum with the same quorum state.
	for id, q := range rp.quorumVotes {
		if q.state == candidatesRaftQuorumState && id != change.ID {
			fmt.Println(rp.Name(), "ALRD VOTED FOR", id, q.state)
			return RaftStatusOK
		}
	}

	q, exist := rp.quorumVotes[change.ID]
	if exist == false {
		if len(rp.quorumVotes) > 5 {
			// to may voting at once
			return RaftStatusOK
		}
		q = &quorum{
			state:      candidatesRaftQuorumState,
			candidates: change.Candidates,
			votes:      make(map[etf.Pid]int),
		}
		rp.quorumVotes[change.ID] = q
		rp.CastAfter(rp.Self(), messageRaftQuorumCleanVote{id: change.ID}, cleanVoteTimeout)
	}

	// mark as recv
	v := q.votes[from]
	v |= 2
	q.votes[from] = v

	candidatesMatch := true
	candidatesVoted := true
	validFrom := false
	duplicates := make(map[etf.Pid]bool)
	for _, pid := range q.candidates {
		duplicates[pid] = false
		if pid == rp.Self() {
			continue
		}
		if pid == from {
			validFrom = true
		}
		if _, exist := rp.quorumCandidates[pid]; exist == false {
			// join this candidate
			join := etf.Tuple{
				etf.Atom("$quorum_join"),
				rp.Self(),
			}
			rp.Cast(pid, join)

			// can't join this quorum due to mismatch of candidates list
			candidatesMatch = false
		}
		if v, _ := q.votes[pid]; v != 3 {
			candidatesVoted = false
		}
	}

	if validFrom == false {
		lib.Warning("%s got request from unknown quorum candidate: %#v", rp.Name(), from)
		return RaftStatusOK
	}

	if len(duplicates) != len(q.candidates) {
		lib.Warning("%s got duplicates from %s in the candidates list: %#v", rp.Name(), from, q.candidates)
		// remove this peer from our candidates list
		delete(rp.quorumCandidates, from)
		return RaftStatusOK
	}

	if candidatesMatch == false {
		return RaftStatusOK
	}
	if candidatesVoted == true {
		if rp.quorumState != RaftQuorumStateUnknown {
			// let all prev quorum peers know that this peer is leaving it
			quorumLeave := etf.Tuple{
				etf.Atom("$quorum_leave"),
				rp.Self(),
				etf.Tuple{
					rp.quorum.ID,
				},
			}
			for _, peer := range rp.quorum.Peers {
				if peer == rp.Self() {
					continue
				}
				if peer == from {
					continue
				}
				rp.Cast(peer, quorumLeave)
			}
		}
		// quorum formed
		rp.quorumFormed(candidatesRaftQuorumState, change.ID, change.Candidates)
		return rp.behavior.HandleQuorumChange(rp, rp.quorumState)
	}

	candidatesVoted = true
	for _, pid := range q.candidates {
		if pid == rp.Self() {
			continue // do not send to itself
		}
		v, _ := q.votes[pid]

		// mark as sent
		q.votes[pid] = v | 1
		if v|1 != 3 {
			candidatesVoted = false
		}

		if v&1 > 0 {
			continue // already sent vote to this peer
		}

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
		rp.quorumFormed(candidatesRaftQuorumState, change.ID, change.Candidates)
		return rp.behavior.HandleQuorumChange(rp, rp.quorumState)
	}

	return RaftStatusOK
}

func (rp *RaftProcess) quorumFormed(state RaftQuorumState, id string, candidates []etf.Pid) {
	fmt.Println(rp.Name(), "QUO FORMED ID:", id, state)
	rp.quorumState = state
	rp.quorum.ID = id
	rp.quorum.Peers = candidates
	delete(rp.quorumVotes, id)
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
	var status RaftStatus

	rp := process.State.(*RaftProcess)
	switch m := message.(type) {
	case messageRaftQuorumCleanVote:
		delete(rp.quorumVotes, m.id)
		if rp.quorumState == RaftQuorumStateWait {
			// seems they built quorum without this peer. keep waiting for
			// the quorum change with the leaving or joining another candidate
			break
		}
		if len(rp.quorumVotes) == 0 && rp.quorumState == RaftQuorumStateUnknown {
			// make another attempt to build new quorum
			after := time.Duration(50+rand.Intn(450)) * time.Millisecond
			rp.CastAfter(rp.Self(), messageRaftQuorumChangeDefer{}, after)
		}
	case messageRaftQuorumChangeDefer:
		status = rp.quorumChange()
	default:
		if err := etf.TermIntoStruct(message, &mRaft); err != nil {
			return rp.behavior.HandleRaftInfo(rp, message)
		}
		status = rp.handleRaftRequest(mRaft)
	}

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
		switch rp.quorumState {
		case RaftQuorumStateWait:
			rp.quorumState = RaftQuorumStateUnknown
		case RaftQuorumStateUnknown:
			break
		default:
			// check if this pid belongs to the quorum
			belongs := false
			for _, peer := range rp.quorum.Peers {
				if peer == m.Pid {
					belongs = true
					break
				}
			}
			if belongs {
				fmt.Println(rp.Name(), "QUO PEER DOWN", m.Pid)
				rp.quorumState = RaftQuorumStateUnknown
				after := time.Duration(50+rand.Intn(450)) * time.Millisecond
				rp.CastAfter(rp.Self(), messageRaftQuorumChangeDefer{}, after)
			}

		}
		return ServerStatusOK

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

// HandleQuorumChange
func (r *Raft) HandleQuorumChange(process *RaftProcess, qs RaftQuorumState) RaftStatus {
	return RaftStatusOK
}

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
