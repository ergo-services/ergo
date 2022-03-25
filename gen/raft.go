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

	// HandleLeaderChange
	HandleLeaderChange(process *RaftProcess, leader etf.Pid) RaftStatus

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

	cleanVoteTimeout         = 1 * time.Second
	quorumChangeDeferMaxTime = 450 // in millisecond. uses as max value in range of 50..
	quorumChangeAttempt      = 0
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
	monitor etf.Ref
	serial  uint64
}

type Quorum struct {
	Member bool
	State  RaftQuorumState
	Peers  []etf.Pid // the number of participants in quorum could be 3,5,7,9,11
}
type quorum struct {
	Quorum
	votes    map[etf.Pid]int // 1 - sent, 2 - recv, 3 - sent and recv
	origin   etf.Pid         // where the voting has come from. it must receive our voice in the last order
	lastVote int64           // time.Now().UnixMilli()
}

type RaftOptions struct {
	ID     string // raft cluster id
	Peers  []ProcessID
	Serial uint64 // serial number. using for leader election
}

type messageRaft struct {
	Request etf.Atom
	Pid     etf.Pid
	Command interface{}
}

type messageRaftQuorumInit struct{}
type messageRaftQuorumJoin struct {
	ID     string
	Serial uint64
}
type messageRaftQuorumReply struct {
	ID          string
	Serial      uint64
	Peers       []etf.Pid
	QuorumState int
	QuorumPeers []etf.Pid
}
type messageRaftQuorumVote struct {
	ID         string
	Serial     uint64
	State      int
	Candidates []etf.Pid
}
type messageRaftQuorumChange struct{}
type messageRaftQuorumBuilt struct {
	ID    string
	State int
	Peers []etf.Pid
}
type messageRaftQuorumCleanVote struct {
	state RaftQuorumState
}

type messageRaftLeaderChange struct{}
type messageRaftLeaderVote struct {
	ID    string
	State int
	Vote  []etf.Pid // quorum members ordered by priority
}

//
// RaftProcess quorum routines and APIs
//

func (rp *RaftProcess) Quorum() Quorum {
	var q Quorum
	q = rp.quorum
	q.Peers = make([]etf.Pid, len(rp.quorum.Peers))
	for i := range rp.quorum.Peers {
		q.Peers[i] = rp.quorum.Peers[i]
	}
	return q
}

func (rp *RaftProcess) handleRaftRequest(m messageRaft) error {
	switch m.Request {
	case etf.Atom("$quorum_join"):
		join := &messageRaftQuorumJoin{}
		if err := etf.TermIntoStruct(m.Command, &join); err != nil {
			return ErrUnsupportedRequest
		}

		if join.ID != rp.options.ID {
			// this peer belongs to another quorum id
			return RaftStatusOK
		}

		rp.quorumCandidates.Add(rp, m.Pid, join.Serial)

		// send peer list even if this peer is already present in our candidates list
		// just to exchange updated data
		peers := rp.quorumCandidates.List()
		reply := etf.Tuple{
			etf.Atom("$quorum_join_reply"),
			rp.Self(),
			etf.Tuple{
				rp.options.ID,
				rp.options.Serial,
				peers,
				int(rp.quorum.State),
				rp.quorum.Peers,
			},
		}
		rp.Cast(m.Pid, reply)
		fmt.Println(rp.Name(), "GOT QUO JOIN from", m.Pid, "send peers", peers)
		return RaftStatusOK

	case etf.Atom("$quorum_join_reply"):

		reply := &messageRaftQuorumReply{}
		if err := etf.TermIntoStruct(m.Command, &reply); err != nil {
			return ErrUnsupportedRequest
		}

		if reply.ID != rp.options.ID {
			// this peer belongs to another quorum id. ignore it.
			return RaftStatusOK
		}

		fmt.Println(rp.Name(), "GOT QUO JOIN REPL from", m.Pid, "got peers", reply.Peers)
		canAcceptQuorum := true
		for _, peer := range reply.Peers {
			if peer == rp.Self() {
				continue
			}
			// check if we dont have some of them among the candidates
			if _, exist := rp.quorumCandidates.Get(peer); exist {
				continue
			}
			rp.quorumJoin(peer)
			canAcceptQuorum = false
		}

		if rp.quorumCandidates.Add(rp, m.Pid, reply.Serial) == false {
			// already present as a candidate
			return RaftStatusOK
		}

		// try to rebuild quorum since the number of peers has changed
		if rp.quorumChangeDefer == false {
			quorumChangeAttempt = 1
			maxTime := quorumChangeAttempt * quorumChangeDeferMaxTime
			after := time.Duration(50+rand.Intn(maxTime)) * time.Millisecond
			rp.CastAfter(rp.Self(), messageRaftQuorumChange{}, after)
			rp.quorumChangeDefer = true
		}

		// accept quorum if this peer is belongs to the existing quorum
		// and set membership to false
		if canAcceptQuorum == true && RaftQuorumState(reply.QuorumState) != RaftQuorumStateUnknown {
			rp.quorum.State = RaftQuorumState(reply.QuorumState)
			rp.quorum.Peers = reply.QuorumPeers
			rp.quorum.Member = false
			return rp.behavior.HandleQuorumChange(rp, rp.quorum.State)
		}

		return RaftStatusOK

	case etf.Atom("$quorum_vote"):
		vote := &messageRaftQuorumVote{}
		if err := etf.TermIntoStruct(m.Command, &vote); err != nil {
			return ErrUnsupportedRequest
		}
		if vote.ID != rp.options.ID {
			// ignore this request
			return RaftStatusOK
		}
		return rp.quorumVote(m.Pid, vote)

	case etf.Atom("$quorum_built"):
		built := &messageRaftQuorumBuilt{}
		if err := etf.TermIntoStruct(m.Command, &built); err != nil {
			return ErrUnsupportedRequest
		}
		fmt.Println(rp.Name(), "GOT QUO BUILT from", m.Pid)
		if built.ID != rp.options.ID {
			// this process is not belong this quorum
			return RaftStatusOK
		}
		duplicates := make(map[etf.Pid]bool)
		matchCandidates := true
		for _, pid := range built.Peers {
			if _, exist := duplicates[pid]; exist {
				// duplicate found
				return RaftStatusOK
			}
			if pid == rp.Self() {
				panic("raft internal error. got quorum built message")
			}
			if _, exist := rp.quorumCandidates.Get(pid); exist {
				continue
			}
			rp.quorumJoin(pid)
			matchCandidates = false
		}
		if len(built.Peers) != built.State {
			// ignore wrong peer list
			lib.Warning("[%s] got quorum state doesn't match with the peer list", rp.Self())
			return RaftStatusOK
		}
		candidateQuorumState := RaftQuorumStateUnknown
		switch built.State {
		case 11:
			candidateQuorumState = RaftQuorumState11
		case 9:
			candidateQuorumState = RaftQuorumState9
		case 7:
			candidateQuorumState = RaftQuorumState7
		case 5:
			candidateQuorumState = RaftQuorumState5
		case 3:
			candidateQuorumState = RaftQuorumState3
		default:
			// ignore wrong state
			return RaftStatusOK
		}

		if rp.quorumChangeDefer == false {
			quorumChangeAttempt = 1
			maxTime := quorumChangeAttempt * quorumChangeDeferMaxTime
			after := time.Duration(50+rand.Intn(maxTime)) * time.Millisecond
			rp.CastAfter(rp.Self(), messageRaftQuorumChange{}, after)
			rp.quorumChangeDefer = true
		}

		// we do accept quorum if it was built using
		// the peers we got registered as candidates
		if matchCandidates == true {
			fmt.Println(rp.Name(), "QUO BUILT. NOT A MEMBER", rp.quorum.State, rp.quorum.Peers)
			changed := false
			if rp.quorum.State != candidateQuorumState {
				changed = true
			}
			rp.quorum.State = candidateQuorumState
			if rp.quorum.Member != false {
				changed = true
			}
			rp.quorum.Member = false
			rp.quorum.Peers = built.Peers
			if changed == true {
				return rp.behavior.HandleQuorumChange(rp, rp.quorum.State)
			}
			return RaftStatusOK
		}

		if rp.quorum.State != RaftQuorumStateUnknown {
			rp.quorum.State = RaftQuorumStateUnknown
			rp.quorum.Member = false
			rp.quorum.Peers = nil
			return rp.behavior.HandleQuorumChange(rp, rp.quorum.State)
		}
	}

	return ErrUnsupportedRequest
}

func (rp *RaftProcess) quorumJoin(peer interface{}) {
	fmt.Println(rp.Name(), "send join to", peer)
	join := etf.Tuple{
		etf.Atom("$quorum_join"),
		rp.Self(),
		etf.Tuple{
			rp.options.ID,
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
			rp.quorum.Member = false
			rp.quorum.Peers = nil
			return rp.behavior.HandleQuorumChange(rp, RaftQuorumStateUnknown)
		}
		fmt.Println(rp.Name(), "QUO VOTE. NOT ENO CAND", rp.quorumCandidates.List())

		// try send join_quorum again to receive an updated peer list
		rp.CastAfter(rp.Self(), messageRaftQuorumInit{}, 5*time.Second)
		return RaftStatusOK
	}

	if _, exist := rp.quorumVotes[candidateRaftQuorumState]; exist {
		// voting for this state is already in progress
		return RaftStatusOK
	}

	quorumCandidates := make([]etf.Pid, 0, l+1)
	quorumCandidates = append(quorumCandidates, rp.Self())
	candidates := rp.quorumCandidates.List()
	quorumCandidates = append(quorumCandidates, candidates[:l]...)
	fmt.Println(rp.Name(), "QUO VOTE INIT", candidateRaftQuorumState, quorumCandidates)

	// send quorumVote to all candidates (except itself)
	quorum := &quorum{
		votes:  make(map[etf.Pid]int),
		origin: rp.Self(),
	}
	quorum.State = candidateRaftQuorumState
	quorum.Peers = quorumCandidates
	rp.quorumVotes[candidateRaftQuorumState] = quorum
	rp.quorumSendVote(quorum)
	rp.CastAfter(rp.Self(), messageRaftQuorumCleanVote{state: quorum.State}, cleanVoteTimeout)
	return RaftStatusOK
}

func (rp *RaftProcess) quorumSendVote(q *quorum) bool {
	empty := etf.Pid{}
	if q.origin == empty {
		// do not send its vote until the origin vote will be received
		return false
	}

	allVoted := true
	quorumVote := etf.Tuple{
		etf.Atom("$quorum_vote"),
		rp.Self(),
		etf.Tuple{
			rp.options.ID,
			rp.options.Serial,
			int(q.State),
			q.Peers,
		},
	}

	for _, pid := range q.Peers {
		if pid == rp.Self() {
			continue // do not send to itself
		}

		if pid == q.origin {
			continue
		}
		v, _ := q.votes[pid]

		// check if already sent vote to this peer
		if v&1 == 0 {
			fmt.Println(rp.Name(), "SEND VOTE to", pid, q.Peers)
			rp.Cast(pid, quorumVote)
			// mark as sent
			v |= 1
			q.votes[pid] = v
		}

		if v != 3 { // 2(010) - recv, 1(001) - sent, 3(011) - recv & sent
			allVoted = false
		}
	}

	if allVoted == true && q.origin != rp.Self() {
		// send vote to origin
		fmt.Println(rp.Name(), "SEND VOTE to origin", q.origin, q.Peers)
		rp.Cast(q.origin, quorumVote)
	}

	return allVoted
}

func (rp *RaftProcess) quorumVote(from etf.Pid, vote *messageRaftQuorumVote) RaftStatus {
	if vote.State != len(vote.Candidates) {
		lib.Warning("[%s] quorum state and number of candidates are mismatch. removing %s from quorum candidates list", rp.Self(), from)
		rp.quorumCandidates.Remove(rp, from, etf.Ref{})
		return RaftStatusOK
	}

	if _, exist := rp.quorumCandidates.Get(from); exist == false {
		// there is a race conditioned case when we received a vote before
		// the quorum_join_reply message. just ignore it. they will start
		// another round of quorum forming
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
	if rp.quorum.State != RaftQuorumStateUnknown && candidatesRaftQuorumState <= rp.quorum.State {
		// There is a case when a peer is involved in more than one voting,
		// and this peer just sent a vote for another voting process which
		// is still in progress.
		// Do not send $quorum_voted message if this peer is already a member
		// of accepted quorum
		member := false
		for _, pid := range rp.quorum.Peers {
			if pid == from {
				member = true
				break
			}
		}
		if member == true {
			return RaftStatusOK
		}

		fmt.Println(rp.Name(), "SKIP VOTE from", from, candidatesRaftQuorumState, rp.quorum.State)
		built := etf.Tuple{
			etf.Atom("$quorum_built"),
			rp.Self(),
			etf.Tuple{
				rp.options.ID,
				int(rp.quorum.State),
				rp.quorum.Peers,
			},
		}
		rp.Cast(from, built)
		return RaftStatusOK
	}

	q, exist := rp.quorumVotes[candidatesRaftQuorumState]
	if exist == false {
		//
		// Received the first vote
		//
		if len(rp.quorumVotes) > 5 {
			// can't be more than 5 (there could be only votes for 3,5,7,9,11)
			lib.Warning("[%s] too many votes %#v", rp.quorumVotes)
			return RaftStatusOK
		}

		q = &quorum{}
		q.State = candidatesRaftQuorumState
		q.Peers = vote.Candidates

		if from == vote.Candidates[0] {
			// Origin vote (received from the peer initiated this voting process).
			// Otherwise keep this field empty, which means this quorum
			// will be overwritten if we get another voting from the peer
			// initiated that voting (with a different set/order of peers)
			q.origin = from
		}

		if rp.quorumValidateVote(from, q, vote) == false {
			// do not create this voting if those peers aren't valid (haven't registered yet)
			return RaftStatusOK
		}
		q.lastVote = time.Now().UnixMilli()
		fmt.Println(rp.Name(), "QUO VOTE (NEW)", from, vote)
		rp.quorumVotes[candidatesRaftQuorumState] = q
		rp.CastAfter(rp.Self(), messageRaftQuorumCleanVote{state: q.State}, cleanVoteTimeout)

	} else {
		empty := etf.Pid{}
		if q.origin == empty && from == vote.Candidates[0] {
			// got origin vote.
			q.origin = from

			// check if this vote has the same set of peers
			same := true
			for i := range q.Peers {
				if vote.Candidates[i] != q.Peers[i] {
					same = false
					break
				}
			}
			// if it differs overwrite quorum by the new voting
			if same == false {
				q.Peers = vote.Candidates
				q.votes = nil
			}
		}

		if rp.quorumValidateVote(from, q, vote) == false {
			return RaftStatusOK
		}
		q.lastVote = time.Now().UnixMilli()
		fmt.Println(rp.Name(), "QUO VOTE", from, vote)
	}

	// returns true if we got votes from all the peers whithin this quorum
	if rp.quorumSendVote(q) == true {
		//
		// Quorum built
		//
		fmt.Println(rp.Name(), "QUO BUILT", q.State, q.Peers)
		rp.quorum.Member = true
		rp.quorum.State = q.State
		rp.quorum.Peers = q.Peers
		delete(rp.quorumVotes, q.State)

		// all candidates who don't belong to this quorum should be known that quorum is built.
		mapPeers := make(map[etf.Pid]bool)
		for _, peer := range rp.quorum.Peers {
			mapPeers[peer] = true
		}
		allCandidates := rp.quorumCandidates.List()
		for _, peer := range allCandidates {
			if _, exist := mapPeers[peer]; exist {
				// this peer belongs to the quorum. skip it
				continue
			}
			built := etf.Tuple{
				etf.Atom("$quorum_built"),
				rp.Self(),
				etf.Tuple{
					rp.options.ID,
					int(rp.quorum.State),
					rp.quorum.Peers,
				},
			}
			rp.Cast(peer, built)

		}

		after := 300 * time.Millisecond
		rp.CastAfter(rp.Self(), messageRaftLeaderChange{}, after)
		return rp.behavior.HandleQuorumChange(rp, rp.quorum.State)
	}

	return RaftStatusOK
}

func (rp *RaftProcess) quorumValidateVote(from etf.Pid, q *quorum, vote *messageRaftQuorumVote) bool {
	duplicates := make(map[etf.Pid]bool)
	validFrom := false
	validTo := false
	validSerial := false
	candidatesMatch := true
	newVote := false
	if q.votes == nil {
		q.votes = make(map[etf.Pid]int)
		newVote = true
	}

	empty := etf.Pid{}
	if q.origin != empty && newVote == true && vote.Candidates[0] != from {
		return false
	}

	for i, pid := range vote.Candidates {
		if pid == rp.Self() {
			validTo = true
			continue
		}

		// quorum peers must be matched with the vote's cadidates
		if q.Peers[i] != vote.Candidates[i] {
			candidatesMatch = false
		}

		// check if received vote has the same set of peers.
		// if this is the first vote for the given q.State the pid
		// will be added to the vote map
		_, exist := q.votes[pid]
		if exist == false {
			if newVote {
				q.votes[pid] = 0
			} else {
				candidatesMatch = false
			}
		}

		if _, exist := duplicates[pid]; exist {
			lib.Warning("[%s] got vote with duplicates from %s", rp.Name(), from)
			rp.quorumCandidates.Remove(rp, from, etf.Ref{})
			return false
		}
		duplicates[pid] = false

		c, exist := rp.quorumCandidates.Get(pid)
		if exist == false {
			candidatesMatch = false
			rp.quorumJoin(pid)
			continue
		}
		if pid == from {
			if c.serial > vote.Serial {
				// invalid serial
				continue
			}
			c.serial = vote.Serial
			validFrom = true
			validSerial = true
		}
	}

	if candidatesMatch == false {
		// can't accept this vote
		fmt.Println(rp.Name(), "QUO CAND MISMATCH", from, vote.Candidates)
		return false
	}

	if validSerial == false {
		lib.Warning("[%s] got vote from %s with invalid serial", rp.Name(), from)
		rp.quorumCandidates.Remove(rp, from, etf.Ref{})
		return false
	}

	if validFrom == false || validTo == false {
		lib.Warning("[%s] got vote from %s with invalid data", rp.Name(), from)
		rp.quorumCandidates.Remove(rp, from, etf.Ref{})
		return false
	}

	// mark as recv
	v, _ := q.votes[from]
	q.votes[from] = v | 2

	return true
}

func (rp *RaftProcess) leaderChange() RaftStatus {
	if rp.quorum.State == RaftQuorumStateUnknown {
		return RaftStatusOK
	}

	return RaftStatusOK
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

	raftProcess.options = options
	process.State = raftProcess

	process.Cast(process.Self(), messageRaftQuorumInit{})
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
	case messageRaftQuorumInit:
		if rp.quorum.State != RaftQuorumStateUnknown {
			return ServerStatusOK
		}
		if len(rp.quorumVotes) > 0 {
			return ServerStatusOK
		}
		for _, peer := range rp.options.Peers {
			rp.quorumJoin(peer)
		}
		return ServerStatusOK

	case messageRaftQuorumCleanVote:
		q, exist := rp.quorumVotes[m.state]
		if exist == true && q.lastVote > 0 {
			diff := time.Duration(time.Now().UnixMilli()-q.lastVote) * time.Millisecond
			// if voting is still in progress cast itself again with shifted timeout
			// according to cleanVoteTimeout
			if cleanVoteTimeout > diff {
				nextCleanVoteTimeout := cleanVoteTimeout - diff
				rp.CastAfter(rp.Self(), messageRaftQuorumCleanVote{state: q.State}, nextCleanVoteTimeout)
				return ServerStatusOK
			}
		}

		// TODO remove debug print
		if q != nil {
			fmt.Println(rp.Name(), "CLN VOTE", m.state, q.Peers)
		}
		delete(rp.quorumVotes, m.state)
		if len(rp.quorumVotes) == 0 {
			// increase timeout for the next attempt to build a new quorum
			quorumChangeAttempt++
			maxTime := quorumChangeAttempt * quorumChangeDeferMaxTime

			// make another attempt to build new quorum
			after := time.Duration(50+rand.Intn(maxTime)) * time.Millisecond
			rp.CastAfter(rp.Self(), messageRaftQuorumChange{}, after)
			rp.quorumChangeDefer = true
		}
	case messageRaftQuorumChange:
		rp.quorumChangeDefer = false
		status = rp.quorumChange()
	case messageRaftLeaderChange:
		status = rp.leaderChange()
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
				quorumChangeAttempt = 1
				maxTime := quorumChangeAttempt * quorumChangeDeferMaxTime
				after := time.Duration(50+rand.Intn(maxTime)) * time.Millisecond
				rp.CastAfter(rp.Self(), messageRaftQuorumChange{}, after)
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

// HandleLeaderChange
func (r *Raft) HandleLeaderChange(process *RaftProcess, leader etf.Pid) RaftStatus {
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

func (qc *quorumCandidates) Add(rp *RaftProcess, peer etf.Pid, serial uint64) bool {
	if _, exist := qc.candidates[peer]; exist {
		return false
	}

	mon := rp.MonitorProcess(peer)
	c := &candidate{
		monitor: mon,
		serial:  serial,
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

func (qc *quorumCandidates) Get(peer etf.Pid) (*candidate, bool) {
	c, exist := qc.candidates[peer]
	return c, exist
}

func (qc *quorumCandidates) List() []etf.Pid {
	type c struct {
		pid    etf.Pid
		serial uint64
	}
	list := []c{}
	for k, v := range qc.candidates {
		list = append(list, c{pid: k, serial: v.serial})
	}

	// sort candidates by serial number in desc order
	sort.Slice(list, func(a, b int) bool { return list[a].serial > list[b].serial })
	pids := []etf.Pid{}
	for i := range list {
		pids = append(pids, list[i].pid)
	}
	return pids
}
