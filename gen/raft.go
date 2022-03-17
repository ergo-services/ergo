package gen

import (
	"fmt"
	"math/rand"
	"sort"
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

	quorum            Quorum
	quorumCandidates  *quorumCandidates
	quorumVotes       map[RaftQuorumState]*quorum
	quorumChangeDefer bool
}

type quorumCandidates struct {
	candidates map[etf.Pid]*candidate
}

type candidate struct {
	monitor    etf.Ref
	lastUpdate int64
}

type Quorum struct {
	Follow bool // sets to 'true' if this quorum was build without this peer
	State  RaftQuorumState
	Peers  []etf.Pid // the number of participants in quorum could be 3,5,7,9,11
}
type quorum struct {
	Quorum
	votes map[etf.Pid]int // 1 - sent, 2 - recv, 3 - sent and recv
}

type RaftOptions struct {
	Peer       ProcessID
	Data       etf.Term
	LastUpdate int64
	QuorumID   string
}

type messageRaft struct {
	Request etf.Atom
	Pid     etf.Pid
	Command interface{}
}

type messageRaftQuorumJoin struct {
	ID         string
	LastUpdate int64
}
type messageRaftQuorumReply struct {
	ID         string
	LastUpdate int64
	Peers      []etf.Pid
}
type messageRaftQuorumVote struct {
	ID         string
	State      int
	Candidates []etf.Pid
}
type messageRaftQuorumChangeDefer struct{}
type messageRaftQuorumLeave struct {
	ID    string
	State int
}
type messageRaftQuorumFollow struct {
	ID    string
	State int
	Peers []etf.Pid
}
type messageRaftQuorumCleanVote struct {
	state RaftQuorumState
}

//
// RaftProcess quorum routines and APIs
//

func (rp *RaftProcess) handleRaftRequest(m messageRaft) error {
	switch m.Request {
	case etf.Atom("$quorum_join"):
		join := &messageRaftQuorumJoin{}
		if err := etf.TermIntoStruct(m.Command, &join); err != nil {
			return ErrUnsupportedRequest
		}

		if join.ID != rp.options.QuorumID {
			// this peer belongs to another quorum id
			return RaftStatusOK
		}

		peers := rp.quorumCandidates.List()
		if rp.quorumCandidates.Add(rp, m.Pid, join.LastUpdate) == false {
			return RaftStatusOK
		}

		reply := etf.Tuple{
			etf.Atom("$quorum_join_reply"),
			rp.Self(),
			etf.Tuple{
				rp.options.QuorumID,
				rp.options.LastUpdate,
				peers,
			},
		}
		rp.Cast(m.Pid, reply)
		fmt.Println(rp.Name(), "GOT QUO JOIN from", m.Pid, peers)
		return RaftStatusOK

	case etf.Atom("$quorum_join_reply"):

		reply := &messageRaftQuorumReply{}
		if err := etf.TermIntoStruct(m.Command, &reply); err != nil {
			return ErrUnsupportedRequest
		}

		if reply.ID != rp.options.QuorumID {
			// this peer belongs to another quorum id
			return RaftStatusOK
		}

		if rp.quorumCandidates.Add(rp, m.Pid, reply.LastUpdate) == false {
			return RaftStatusOK
		}

		fmt.Println(rp.Name(), "GOT QUO JOIN REPL from", m.Pid, "send peers", reply.Peers)
		for _, peer := range reply.Peers {
			if peer == rp.Self() {
				continue
			}
			// check if we dont have some of them among the candidates
			if _, exist := rp.quorumCandidates.Get(peer); exist {
				continue
			}
			rp.quorumJoin(peer)
		}

		if rp.quorumChangeDefer == false {
			after := time.Duration(50+rand.Intn(450)) * time.Millisecond
			rp.CastAfter(rp.Self(), messageRaftQuorumChangeDefer{}, after)
			rp.quorumChangeDefer = true
		}
		return RaftStatusOK

	case etf.Atom("$quorum_vote"):
		vote := &messageRaftQuorumVote{}
		if err := etf.TermIntoStruct(m.Command, &vote); err != nil {
			return ErrUnsupportedRequest
		}
		if vote.ID != rp.options.QuorumID {
			// ignore this request
			return RaftStatusOK
		}
		return rp.quorumVote(m.Pid, vote)

	case etf.Atom("$quorum_leave"):
		leave := &messageRaftQuorumLeave{}
		if err := etf.TermIntoStruct(m.Command, &leave); err != nil {
			return ErrUnsupportedRequest
		}

		if leave.ID != rp.options.QuorumID {
			// this process is not belong this quorum
			return RaftStatusOK
		}

		fmt.Println(rp.Name(), "LEAV QUO", rp.options.QuorumID, m.Pid)
		rp.quorum.State = RaftQuorumStateUnknown
		status := rp.behavior.HandleQuorumChange(rp, rp.quorum.State)

		if len(rp.quorumVotes) > 0 {
			// voting is in progress
			return status
		}

		if rp.quorumChangeDefer == false {
			after := time.Duration(50+rand.Intn(450)) * time.Millisecond
			rp.CastAfter(rp.Self(), messageRaftQuorumChangeDefer{}, after)
			rp.quorumChangeDefer = true
		}
		return status

	case etf.Atom("$quorum_formed"):
		if rp.quorum.Follow {
			return RaftStatusOK
		}
		follow := &messageRaftQuorumFollow{}
		if err := etf.TermIntoStruct(m.Command, &follow); err != nil {
			return ErrUnsupportedRequest
		}
		if follow.ID != rp.options.QuorumID {
			// this process is not belong this quorum
			return RaftStatusOK
		}
		duplicates := make(map[etf.Pid]bool)
		for _, pid := range follow.Peers {
			if _, exist := duplicates[pid]; exist {
				// duplicate found
				return RaftStatusOK
			}
			if pid == rp.Self() {
				continue
			}
			if _, exist := rp.quorumCandidates.Get(pid); exist {
				continue
			}
			rp.quorumJoin(pid)
		}
		if len(follow.Peers) != follow.State {
			// ignore wrong peer list
			return RaftStatusOK
		}
		switch follow.State {
		case 11:
			rp.quorum.State = RaftQuorumState11
		case 9:
			rp.quorum.State = RaftQuorumState9
		case 7:
			rp.quorum.State = RaftQuorumState7
		case 5:
			rp.quorum.State = RaftQuorumState5
		case 3:
			rp.quorum.State = RaftQuorumState3
		default:
			// ignore wrong state
			return RaftStatusOK
		}
		rp.quorum.Follow = true
		rp.quorum.Peers = follow.Peers
		fmt.Println(rp.Name(), "QUO FOLLOWER", rp.quorum.State, rp.quorum.Peers)
		return RaftStatusOK
	}

	return ErrUnsupportedRequest
}

func (rp *RaftProcess) quorumJoin(peer interface{}) {
	join := etf.Tuple{
		etf.Atom("$quorum_join"),
		rp.Self(),
		etf.Tuple{
			rp.options.QuorumID,
		},
	}
	rp.Cast(peer, join)
}

func (rp *RaftProcess) quorumChange() RaftStatus {
	l := rp.quorumCandidates.Len()
	candidateRaftQuorumState := RaftQuorumStateUnknown
	switch {
	case l > 9:
		if rp.quorum.State == RaftQuorumState11 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState11
		l = 10 // to create quorum of 11 we need 10 candidates + itself.

	case l > 7:
		if rp.quorum.State == RaftQuorumState9 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState9
		l = 8 // quorum of 9 => 8 candidates + itself
	case l > 5:
		if rp.quorum.State == RaftQuorumState7 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState7
		l = 6 // quorum of 7 => 6 candidates + itself
	case l > 3:
		if rp.quorum.State == RaftQuorumState5 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState5
		l = 4 // quorum of 5 => 4 candidates + itself
	case l > 1:
		if rp.quorum.State == RaftQuorumState3 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState3
		l = 2 // quorum of 3 => 2 candidates + itself
	default:
		// not enougth candidates to create a quorum
		if rp.quorum.State != RaftQuorumStateUnknown {
			rp.quorum.State = RaftQuorumStateUnknown
			return rp.behavior.HandleQuorumChange(rp, RaftQuorumStateUnknown)
		}
		fmt.Println(rp.Name(), "QUO VOTE. NOT ENO CAND", rp.quorumCandidates.List())
		return RaftStatusOK
	}

	quorumCandidates := make([]etf.Pid, 0, l+1)
	quorumCandidates = append(quorumCandidates, rp.Self())
	candidates := rp.quorumCandidates.List()
	quorumCandidates = append(quorumCandidates, candidates[:l]...)
	fmt.Println(rp.Name(), "QUO VOTE INIT", candidateRaftQuorumState, quorumCandidates)

	// send quorumVote to all candidates
	quorumVote := etf.Tuple{
		etf.Atom("$quorum_vote"),
		rp.Self(),
		etf.Tuple{
			rp.options.QuorumID,
			int(candidateRaftQuorumState),
			quorumCandidates,
		},
	}
	quorum := &quorum{
		votes: make(map[etf.Pid]int),
	}
	quorum.State = candidateRaftQuorumState
	quorum.Peers = quorumCandidates
	// do not send to itself
	for _, pid := range quorumCandidates[1:] {
		fmt.Println(rp.Name(), "SEND QUO VOTE to", pid)
		quorum.votes[pid] = 1
		rp.Cast(pid, quorumVote)
	}
	rp.quorumVotes[candidateRaftQuorumState] = quorum
	rp.CastAfter(rp.Self(), messageRaftQuorumCleanVote{state: quorum.State}, cleanVoteTimeout)
	return RaftStatusOK
}

func (rp *RaftProcess) quorumVote(from etf.Pid, vote *messageRaftQuorumVote) RaftStatus {
	fmt.Println(rp.Name(), "QUO VOTE", from, vote)
	if vote.State != len(vote.Candidates) {
		lib.Warning("[%s] quorum state and number of candidates are mismatch. removing %s from quorum candidates list", rp.Self(), from)
		rp.quorumCandidates.Remove(rp, from, etf.Ref{})
		return RaftStatusOK
	}

	if _, exist := rp.quorumCandidates.Get(from); exist == false {
		lib.Warning("[%s] got vote from unknown peer %s", rp.Self(), from)
		return RaftStatusOK
	}
	candidatesRaftQuorumState := RaftQuorumStateUnknown
	switch vote.State {
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
		rp.quorumCandidates.Remove(rp, from, etf.Ref{})
		return RaftStatusOK
	}

	// do not vote if requested quorum is less than existing one
	if rp.quorum.State != RaftQuorumStateUnknown && vote.State < int(rp.quorum.State)+1 {
		fmt.Println(rp.Name(), "SKIP VOTE from", from)
		if rp.quorum.Follow == false {
			formed := etf.Tuple{
				etf.Atom("$quorum_formed"),
				rp.Self(),
				etf.Tuple{
					rp.options.QuorumID,
					int(rp.quorum.State),
					rp.quorum.Peers,
				},
			}
			rp.Cast(from, formed)
		}
		return RaftStatusOK
	}

	q, exist := rp.quorumVotes[candidatesRaftQuorumState]
	if exist == false {
		//
		// Received the first vote
		//
		if len(rp.quorumVotes) > 5 {
			// to many voting at once
			return RaftStatusOK
		}
		q = &quorum{
			votes: make(map[etf.Pid]int),
		}
		duplicates := make(map[etf.Pid]bool)
		validFrom := false
		validTo := false
		candidatesMatch := true
		for _, pid := range vote.Candidates {
			if pid == rp.Self() {
				validTo = true
				continue
			}
			if _, exist := rp.quorumCandidates.Get(pid); exist == false {
				candidatesMatch = false
				rp.quorumJoin(pid)
			}
			if _, exist := duplicates[pid]; exist {
				lib.Warning("[%s] got vote with duplicates from %s", rp.Name(), from)
				rp.quorumCandidates.Remove(rp, from, etf.Ref{})
				return RaftStatusOK
			}
			duplicates[pid] = false

			if pid == from {
				// mark as recv vote from this peer
				q.votes[pid] = 2
				validFrom = true
				continue
			}
			q.votes[pid] = 0
		}

		if validFrom == false || validTo == false {
			lib.Warning("[%s] got vote from %s with incorrect candidates list", rp.Name(), from)
			rp.quorumCandidates.Remove(rp, from, etf.Ref{})
			return RaftStatusOK
		}

		if candidatesMatch == false {
			// ignore this vote
			return RaftStatusOK
		}

		q.State = candidatesRaftQuorumState
		q.Peers = vote.Candidates

		rp.quorumVotes[candidatesRaftQuorumState] = q
		rp.CastAfter(rp.Self(), messageRaftQuorumCleanVote{state: q.State}, cleanVoteTimeout)

	} else {
		// candidates list must be matched with the list of quorum peers
		// and shouldn't have duplicates
		duplicates := make(map[etf.Pid]bool)
		validFrom := false
		for _, pid := range vote.Candidates {
			if pid == rp.Self() {
				continue
			}
			// check for duplicate
			if _, exist := duplicates[pid]; exist {
				lib.Warning("[%s] got vote with duplicates from %s: %#v", rp.Name(), from, vote.Candidates)
				rp.quorumCandidates.Remove(rp, from, etf.Ref{})
			}
			duplicates[pid] = false

			// 'from' must be a part of the provided candidates list

			v, exist := q.votes[pid]
			if exist == false {
				// mismatch. ignore this vote
				fmt.Println("QUO VOTE MISMATCH")
				return RaftStatusOK
			}
			if pid == from {
				validFrom = true
				// mark as recv
				q.votes[from] = v | 2
			}
		}

		if validFrom == false {
			lib.Warning("[%s] got vote from %s which doesn't belongs offered quorum", rp.Name(), from)
			rp.quorumCandidates.Remove(rp, from, etf.Ref{})
			return RaftStatusOK
		}
	}

	candidatesVoted := true
	for _, pid := range q.Peers {
		if pid == rp.Self() {
			continue // do not send to itself
		}
		v, _ := q.votes[pid]

		// mark as sent
		if v|1 != 3 {
			candidatesVoted = false
		}

		// check if already sent vote to this peer
		if v&1 > 0 {
			continue
		}

		q.votes[pid] = v | 1
		// send quorum change request to the others
		quorumVote := etf.Tuple{
			etf.Atom("$quorum_vote"),
			rp.Self(),
			etf.Tuple{
				rp.options.QuorumID,
				vote.State,
				vote.Candidates,
			},
		}
		fmt.Println(rp.Name(), "SEND QUO VOTE to", pid)
		rp.Cast(pid, quorumVote)

	}

	if candidatesVoted == true {
		//
		// Quorum formed
		//
		if rp.quorum.State != RaftQuorumStateUnknown {
			// let all prev quorum peers know that this peer is leaving it
			quorumLeave := etf.Tuple{
				etf.Atom("$quorum_leave"),
				rp.Self(),
				etf.Tuple{
					rp.options.QuorumID,
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
		rp.quorumFormed(q.State, q.Peers)
		return rp.behavior.HandleQuorumChange(rp, rp.quorum.State)
	}

	return RaftStatusOK
}

func (rp *RaftProcess) quorumFormed(state RaftQuorumState, peers []etf.Pid) {
	fmt.Println(rp.Name(), "QUO FORMED STATE:", state, peers)
	rp.quorum.State = state
	rp.quorum.Peers = peers
	delete(rp.quorumVotes, state)
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
		quorumCandidates: createQuorumCandidates(),
		quorumVotes:      make(map[RaftQuorumState]*quorum),
	}

	// do not inherit parent State
	raftProcess.State = nil
	options, err := behavior.InitRaft(raftProcess, args...)
	if err != nil {
		return err
	}

	// LastUpdate can't be > 0 if Data is nil
	// LastUpdate can't be > current time
	// LastUpdate can't be < 0
	if options.Data == nil || options.LastUpdate > time.Now().Unix() || options.LastUpdate < 0 {
		options.LastUpdate = 0
	}

	raftProcess.options = options
	process.State = raftProcess

	noPeer := ProcessID{}
	if options.Peer == noPeer {
		return nil
	}

	raftProcess.quorumJoin(options.Peer)

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
		delete(rp.quorumVotes, m.state)
		if rp.quorum.Follow {
			// seems they built quorum without this peer. keep waiting for
			// the quorum change with the leaving or joining another candidate
			break
		}
		if len(rp.quorumVotes) == 0 && rp.quorum.State == RaftQuorumStateUnknown {
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
		if mRaft.Pid == process.Self() {
			lib.Warning("[%s] got raft command from itself %#v", process.Self(), mRaft)
			return ServerStatusOK
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
		can, exist := rp.quorumCandidates.Get(m.Pid)
		if can.monitor != m.Ref {
			status = rp.behavior.HandleRaftInfo(rp, message)
			break
		}
		if exist == false {
			break
		}
		rp.quorumCandidates.Remove(rp, m.Pid, can.monitor)
		switch rp.quorum.State {
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
				// start to build new quorum
				fmt.Println(rp.Name(), "QUO PEER DOWN", m.Pid)
				rp.quorum.State = RaftQuorumStateUnknown
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

//
// internals
//

func createQuorumCandidates() *quorumCandidates {
	qc := &quorumCandidates{
		candidates: make(map[etf.Pid]*candidate),
	}
	return qc
}

func (qc *quorumCandidates) Add(rp *RaftProcess, peer etf.Pid, lastUpdate int64) bool {
	if _, exist := qc.candidates[peer]; exist {
		return false
	}

	mon := rp.MonitorProcess(peer)
	c := &candidate{
		monitor:    mon,
		lastUpdate: lastUpdate,
	}
	qc.candidates[peer] = c
	return true
}

func (qc *quorumCandidates) Remove(rp *RaftProcess, peer etf.Pid, mon etf.Ref) bool {
	c, exist := qc.candidates[peer]
	if exist == false {
		return false
	}
	emptyRef := etf.Ref{}
	if mon != emptyRef && c.monitor != mon {
		return false
	}
	rp.DemonitorProcess(mon)
	delete(qc.candidates, peer)
	return true
}

func (qc *quorumCandidates) Len() int {
	return len(qc.candidates)
}

func (qc *quorumCandidates) Get(peer etf.Pid) (candidate, bool) {
	var cand candidate
	c, exist := qc.candidates[peer]
	if exist {
		cand = *c
	}
	return cand, exist
}

func (qc *quorumCandidates) List() []etf.Pid {
	type c struct {
		pid etf.Pid
		lu  int64
	}
	list := []c{}
	for k, v := range qc.candidates {
		list = append(list, c{pid: k, lu: v.lastUpdate})
	}
	sort.Slice(list, func(a, b int) bool { return list[a].lu > list[b].lu })
	pids := []etf.Pid{}
	for i := range list {
		pids = append(pids, list[i].pid)
	}
	return pids
}
