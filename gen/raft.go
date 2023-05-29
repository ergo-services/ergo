package gen

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

const (
	DefaultRaftGetTimeout    = 5 // in seconds
	DefaultRaftAppendTimeout = 5 // in seconds
	DefaultRaftHeartbeat     = 3 // in seconds
)

var (
	ErrRaftState        = fmt.Errorf("incorrect raft state")
	ErrRaftNoQuorum     = fmt.Errorf("no quorum")
	ErrRaftNoLeader     = fmt.Errorf("no leader")
	ErrRaftNoSerial     = fmt.Errorf("no peers with requested serial")
	ErrRaftBusy         = fmt.Errorf("another append request is in progress")
	ErrRaftWrongTimeout = fmt.Errorf("wrong timeout value")
)

type RaftBehavior interface {
	ServerBehavior
	//
	// Mandatory callbacks
	//

	InitRaft(process *RaftProcess, arr ...etf.Term) (RaftOptions, error)

	// HandleAppend. Invokes on append request. To cancel this request by a leader, it must return RaftStatusDiscard.
	HandleAppend(process *RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) RaftStatus

	// HandleGet
	HandleGet(process *RaftProcess, serial uint64) (string, etf.Term, RaftStatus)

	//
	// Optional callbacks
	//

	// HandlePeer
	HandlePeer(process *RaftProcess, peer etf.Pid, serial uint64) RaftStatus

	// HandleQuorum
	HandleQuorum(process *RaftProcess, quorum *RaftQuorum) RaftStatus

	// HandleLeader
	HandleLeader(process *RaftProcess, leader *RaftLeader) RaftStatus

	// HandleCancel
	HandleCancel(process *RaftProcess, ref etf.Ref, reason string) RaftStatus

	// HandleSerial
	HandleSerial(process *RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) RaftStatus

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
	RaftStatusOK      RaftStatus // nil
	RaftStatusStop    RaftStatus = fmt.Errorf("stop")
	RaftStatusDiscard RaftStatus = fmt.Errorf("discard")

	RaftQuorumState3  RaftQuorumState = 3 // minimum quorum that could make leader election
	RaftQuorumState5  RaftQuorumState = 5
	RaftQuorumState7  RaftQuorumState = 7
	RaftQuorumState9  RaftQuorumState = 9
	RaftQuorumState11 RaftQuorumState = 11 // maximal quorum

	cleanVoteTimeout         = 1 * time.Second
	cleanLeaderVoteTimeout   = 1 * time.Second
	quorumChangeDeferMaxTime = 450 // in millisecond. uses as max value in range of 50..
)

type Raft struct {
	Server
}

type RaftProcess struct {
	ServerProcess
	options  RaftOptions
	behavior RaftBehavior

	quorum              *RaftQuorum
	quorumCandidates    *quorumCandidates
	quorumVotes         map[RaftQuorumState]*quorum
	quorumChangeDefer   bool
	quorumChangeAttempt int

	leader   etf.Pid
	election *leaderElection
	round    int // "log term" in terms of Raft spec

	// get requests
	requests map[etf.Ref]CancelFunc

	// append requests
	requestsAppend      map[string]*requestAppend
	requestsAppendQueue []requestAppendQueued

	// leader sends heartbeat messages and keep the last sending timestamp
	heartbeatLeader int64
	heartbeatCancel CancelFunc
}

type leaderElection struct {
	votes   map[etf.Pid]etf.Pid
	results map[etf.Pid]bool
	round   int
	leader  etf.Pid // leader elected
	voted   int     // number of peers voted for the leader
	cancel  CancelFunc
}

type requestAppend struct {
	ref    etf.Ref
	from   etf.Pid
	origin etf.Pid
	value  etf.Term
	peers  map[etf.Pid]bool
	cancel CancelFunc
}

type requestAppendQueued struct {
	from    etf.Pid
	request *messageRaftRequestAppend
}

type quorumCandidates struct {
	candidates map[etf.Pid]*candidate
}

type candidate struct {
	monitor   etf.Ref
	serial    uint64
	joined    bool
	heartbeat int64
	failures  int
}

type RaftLeader struct {
	Leader etf.Pid
	Serial uint64
	State  RaftQuorumState
}

type RaftQuorum struct {
	Member bool
	State  RaftQuorumState
	Peers  []etf.Pid // the number of participants in quorum could be 3,5,7,9,11
}
type quorum struct {
	RaftQuorum
	votes    map[etf.Pid]int // 1 - sent, 2 - recv, 3 - sent and recv
	origin   etf.Pid         // where the voting has come from. it must receive our voice in the last order
	lastVote int64           // time.Now().UnixMilli()
}

type RaftOptions struct {
	ID     string // raft cluster id
	Peers  []ProcessID
	Serial uint64 // serial number ("log id" in terms of Raft spec)
}

type messageRaft struct {
	Request etf.Atom
	Pid     etf.Pid
	Command interface{}
}

type messageRaftClusterInit struct{}
type messageRaftClusterJoin struct {
	ID     string // cluster id
	Serial uint64
}
type messageRaftClusterJoinReply struct {
	ID          string // cluster id
	Serial      uint64
	Peers       []etf.Pid
	QuorumState int
	QuorumPeers []etf.Pid
}
type messageRaftQuorumVote struct {
	ID         string // cluster id
	Serial     uint64
	State      int
	Candidates []etf.Pid
}
type messageRaftQuorumChange struct{}
type messageRaftQuorumBuilt struct {
	ID    string // cluster id
	State int
	Round int // last round
	Peers []etf.Pid
}
type messageRaftQuorumLeave struct {
	ID       string
	DueToPid etf.Pid
}

type messageRaftQuorumCleanVote struct {
	state RaftQuorumState
}

type messageRaftLeaderHeartbeat struct {
	ID     string
	Serial uint64
}

type messageRaftLeaderVote struct {
	ID     string  // cluster id
	State  int     //quorum state
	Leader etf.Pid // offered leader
	Round  int
}
type messageRaftLeaderElected struct {
	ID     string  // cluster id
	Leader etf.Pid // elected leader
	Voted  int     // number of votes for this leader
	Round  int
}

type messageRaftRequestGet struct {
	ID     string // cluster id
	Ref    etf.Ref
	Origin etf.Pid
	Serial uint64
}
type messageRaftRequestReply struct {
	ID     string // cluster id
	Ref    etf.Ref
	Serial uint64
	Key    string
	Value  etf.Term
}
type messageRaftRequestAppend struct {
	ID       string // cluster id
	Ref      etf.Ref
	Origin   etf.Pid
	Key      string
	Value    etf.Term
	Deadline int64 // timestamp in milliseconds
}

type messageRaftAppendReady struct {
	ID  string // cluster id
	Ref etf.Ref
	Key string
}

type messageRaftAppendCommit struct {
	ID        string // cluster id
	Ref       etf.Ref
	Key       string
	Serial    uint64
	Broadcast etf.Pid // quorum member who is in charge of broadcasting
}

type messageRaftAppendBroadcast struct {
	ID     string
	Ref    etf.Ref
	Serial uint64
	Key    string
	Value  etf.Term
}

type messageRaftRequestClean struct {
	ref etf.Ref
}
type messageRaftAppendClean struct {
	key string
	ref etf.Ref
}
type messageRaftElectionClean struct {
	round int
}
type messageRaftHeartbeat struct{}

//
// RaftProcess quorum routines and APIs
//

// Join makes a join requst to the given peer, which is supposed to be in a raft cluster
func (rp *RaftProcess) Join(peer interface{}) error {
	// QUODBG fmt.Println(rp.Name(), "CLU send join to", peer)
	join := etf.Tuple{
		etf.Atom("$cluster_join"),
		rp.Self(),
		etf.Tuple{
			rp.options.ID,
		},
	}
	return rp.Cast(peer, join)
}

// Peers returns list of the processes in the raft cluster. Note, this list is sorted by the Serial value on them in the descending order
func (rp *RaftProcess) Peers() []etf.Pid {
	return rp.quorumCandidates.List()
}

// Quorum returns current quorum. It returns nil if quorum hasn't built yet.
func (rp *RaftProcess) Quorum() *RaftQuorum {
	var q RaftQuorum
	if rp.quorum == nil {
		return nil
	}
	q.Member = rp.quorum.Member
	q.State = rp.quorum.State
	q.Peers = make([]etf.Pid, len(rp.quorum.Peers))
	for i := range rp.quorum.Peers {
		q.Peers[i] = rp.quorum.Peers[i]
	}
	return &q
}

// Leader returns current leader in the quorum. It returns nil If this process is not a quorum or if leader election is still in progress
func (rp *RaftProcess) Leader() *RaftLeader {
	var leader RaftLeader

	if rp.quorum == nil || rp.quorum.Member == false {
		return nil
	}

	noLeader := etf.Pid{}
	if rp.leader == noLeader {
		return nil
	}
	leader.Leader = rp.leader
	leader.State = rp.quorum.State
	leader.Serial = rp.options.Serial
	if rp.leader != rp.Self() {
		// must be present among the peers
		c := rp.quorumCandidates.GetOnline(rp.leader)
		if c == nil {
			panic("internal error. elected leader has been lost")
		}
		leader.Serial = c.serial
	}

	return &leader
}

// Get makes a request to the quorum member to get the data with the given serial number and
// sets the timeout to the DefaultRaftGetTimeout = 5 sec. It returns ErrRaftNoQuorum if quorum
// forming is still in progress.
func (rp *RaftProcess) Get(serial uint64) (etf.Ref, error) {
	return rp.GetWithTimeout(serial, DefaultRaftGetTimeout)
}

// Get makes a request to the quorum member to get the data with the given serial number and
// timeout in seconds. Returns a reference of this request. Once requested data has arrived
// the callback HandleSerial will be invoked.
// If a timeout occurred the callback HandleCancel will be invoked with reason "timeout"
func (rp *RaftProcess) GetWithTimeout(serial uint64, timeout int) (etf.Ref, error) {
	var ref etf.Ref
	if rp.quorum == nil {
		return ref, ErrRaftNoQuorum
	}

	peers := []etf.Pid{}
	for _, pid := range rp.quorum.Peers {
		if pid == rp.Self() {
			continue
		}
		if c := rp.quorumCandidates.GetOnline(pid); c != nil {
			if serial > c.serial {
				continue
			}
			peers = append(peers, pid)
		}
	}
	if len(peers) == 0 {
		return ref, ErrRaftNoSerial
	}

	// get random member of quorum and send the request
	n := 0
	if len(peers) > 1 {
		rand.Intn(len(peers) - 1)
	}
	peer := peers[n]
	ref = rp.MakeRef()
	requestGet := etf.Tuple{
		etf.Atom("$request_get"),
		rp.Self(),
		etf.Tuple{
			rp.options.ID,
			ref,
			rp.Self(), // origin
			serial,
		},
	}

	if err := rp.Cast(peer, requestGet); err != nil {
		return ref, err
	}
	cancel := rp.CastAfter(rp.Self, messageRaftRequestClean{ref: ref}, time.Duration(timeout)*time.Second)
	rp.requests[ref] = cancel
	return ref, nil
}

// Append
func (rp *RaftProcess) Append(key string, value etf.Term) (etf.Ref, error) {
	return rp.AppendWithTimeout(key, value, DefaultRaftAppendTimeout)
}

// AppendWithTimeout
func (rp *RaftProcess) AppendWithTimeout(key string, value etf.Term, timeout int) (etf.Ref, error) {
	var ref etf.Ref
	if timeout < 1 {
		return ref, ErrRaftWrongTimeout
	}

	if _, exist := rp.requestsAppend[key]; exist {
		return ref, ErrRaftBusy
	}
	if rp.quorum == nil {
		return ref, ErrRaftNoQuorum
	}
	noLeader := etf.Pid{}
	if rp.quorum.Member == true && rp.leader == noLeader {
		return ref, ErrRaftNoLeader
	}
	t := int(time.Duration(timeout) * time.Second)
	deadline := time.Now().Add(time.Duration(t - t/int(rp.quorum.State))).UnixMilli()
	ref = rp.MakeRef()

	// if Append request has made on a leader
	if rp.leader == rp.Self() {
		// DBGAPN fmt.Println(rp.Self(), "DBGAPN append request", ref, "made on a leader")
		dataAppend := &messageRaftRequestAppend{
			Ref:      ref,
			Origin:   rp.Self(),
			Key:      key,
			Value:    value,
			Deadline: deadline,
		}
		rp.handleAppendLeader(rp.Self(), dataAppend)
		return ref, nil
	}

	peer := rp.leader
	// if Member == false => rp.leader == noLeader
	if rp.quorum.Member == false {
		// this raft process runs as a Client. send this request to the quorum member
		n := rand.Intn(len(rp.quorum.Peers) - 1)
		peer = rp.quorum.Peers[n]
		deadline = time.Now().Add(time.Duration(t - t/(int(rp.quorum.State)+1))).UnixMilli()
	}
	dataAppend := etf.Tuple{
		etf.Atom("$request_append"),
		rp.Self(),
		etf.Tuple{
			rp.options.ID,
			ref,
			rp.Self(),
			key,
			value,
			deadline,
		},
	}
	// DBGAPN fmt.Println(rp.Self(), "DPGAPN sent $request_append", ref, "to the peer", peer)
	if err := rp.Cast(peer, dataAppend); err != nil {
		return ref, err
	}

	peers := make(map[etf.Pid]bool)
	if rp.quorum.Member == true {
		// this process will be in charge of broadcasting
		// so we should keep the set of peers in this quorum in order
		// to exlude them on the broadcasting
		for _, pid := range rp.quorum.Peers {
			if pid == rp.Self() {
				continue
			}
			peers[pid] = true
		}
	}

	clean := messageRaftAppendClean{key: key, ref: ref}
	after := time.Duration(timeout) * time.Second
	cancel := rp.CastAfter(rp.Self, clean, after)
	requestAppend := &requestAppend{
		ref:    ref,
		origin: rp.Self(),
		value:  value,
		peers:  peers,
		cancel: cancel,
	}
	rp.requestsAppend[key] = requestAppend
	return ref, nil
}

// Serial returns current value of serial for this raft process
func (rp *RaftProcess) Serial() uint64 {
	return rp.options.Serial
}

// private routines

func (rp *RaftProcess) handleRaftRequest(m messageRaft) error {
	switch m.Request {
	case etf.Atom("$cluster_join"):
		join := &messageRaftClusterJoin{}
		if err := etf.TermIntoStruct(m.Command, &join); err != nil {
			return lib.ErrUnsupportedRequest
		}

		if join.ID != rp.options.ID {
			// this peer belongs to another quorum id
			return RaftStatusOK
		}

		if rp.quorum != nil && rp.quorum.Member {
			// if we got $cluster_join from a quorum member, it means
			// the quorum we had belonging is not existed anymore
			if rp.isQuorumMember(m.Pid) == true {
				rp.quorum = nil
				rp.handleQuorum()
				rp.quorumChangeStart(false)
			}
		}

		rp.quorumCandidates.Set(rp, m.Pid)
		rp.quorumCandidates.SetOnline(rp, m.Pid, join.Serial)

		if status := rp.behavior.HandlePeer(rp, m.Pid, join.Serial); status != RaftStatusOK {
			return status
		}

		// send peer list even if this peer is already present in our candidates list
		// just to exchange updated data
		peers := rp.quorumCandidates.List()
		quorumState := 0
		quorumPeers := []etf.Pid{}
		if rp.quorum != nil {
			quorumState = int(rp.quorum.State)
			quorumPeers = rp.quorum.Peers
		}
		reply := etf.Tuple{
			etf.Atom("$cluster_join_reply"),
			rp.Self(),
			etf.Tuple{
				rp.options.ID,
				rp.options.Serial,
				peers,
				quorumState,
				quorumPeers,
			},
		}
		// QUODBG fmt.Println(rp.Name(), "GOT CLU JOIN from", m.Pid, "send peers", peers)
		rp.Cast(m.Pid, reply)
		return RaftStatusOK

	case etf.Atom("$cluster_join_reply"):

		reply := &messageRaftClusterJoinReply{}
		if err := etf.TermIntoStruct(m.Command, &reply); err != nil {
			return lib.ErrUnsupportedRequest
		}

		if reply.ID != rp.options.ID {
			// this peer belongs to another quorum id. ignore it.
			return RaftStatusOK
		}

		// QUODBG fmt.Println(rp.Name(), "GOT CLU JOIN REPL from", m.Pid, "got peers", reply.Peers)
		canAcceptQuorum := true

		// check if there is another quorum in this cluster
		if rp.quorum != nil {
			// doesnt matter we compare the number of peers or quorum state
			// reply.QuorumState <= rp.quorum.State
			if len(reply.QuorumPeers) <= len(rp.quorum.Peers) {
				canAcceptQuorum = false
			}
		}

		// check peers
		for _, peer := range reply.Peers {
			if peer == rp.Self() {
				continue
			}
			// check if we dont have some of them among the online peers
			if c := rp.quorumCandidates.GetOnline(peer); c != nil {
				continue
			}
			rp.quorumCandidates.Set(rp, peer)
			canAcceptQuorum = false
		}

		rp.quorumCandidates.Set(rp, m.Pid)
		rp.quorumCandidates.SetOnline(rp, m.Pid, reply.Serial)

		if status := rp.behavior.HandlePeer(rp, m.Pid, reply.Serial); status != RaftStatusOK {
			return status
		}

		// try to rebuild quorum since the number of peers has changed
		rp.quorumChangeStart(false)

		// accept quorum if this peer is belongs to the existing quorum
		// and set membership to false
		switch RaftQuorumState(reply.QuorumState) {
		case RaftQuorumState3, RaftQuorumState5:
			break
		case RaftQuorumState7, RaftQuorumState9, RaftQuorumState11:
			break
		default:
			canAcceptQuorum = false
		}
		if canAcceptQuorum == true {
			rp.election = nil
			rp.quorum = &RaftQuorum{
				State:  RaftQuorumState(reply.QuorumState),
				Peers:  reply.QuorumPeers,
				Member: false,
			}
			return rp.handleQuorum()
		}
		return RaftStatusOK

	case etf.Atom("$quorum_vote"):
		vote := &messageRaftQuorumVote{}
		if err := etf.TermIntoStruct(m.Command, &vote); err != nil {
			return lib.ErrUnsupportedRequest
		}
		if vote.ID != rp.options.ID {
			// ignore this request
			return RaftStatusOK
		}
		return rp.quorumVote(m.Pid, vote)

	case etf.Atom("$quorum_built"):
		built := &messageRaftQuorumBuilt{}
		if err := etf.TermIntoStruct(m.Command, &built); err != nil {
			return lib.ErrUnsupportedRequest
		}
		// QUODBG fmt.Println(rp.Name(), "GOT QUO BUILT from", m.Pid)
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
			if c := rp.quorumCandidates.GetOnline(pid); c != nil {
				c.failures = 0
				c.heartbeat = time.Now().Unix()
				continue
			}
			rp.quorumCandidates.Set(rp, pid)
			matchCandidates = false
		}
		if len(built.Peers) != built.State {
			// ignore wrong peer list
			lib.Warning("[%s] got quorum state doesn't match with the peer list", rp.Self())
			return RaftStatusOK
		}
		candidateQuorumState := RaftQuorumState3
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

		rp.quorumChangeStart(false)

		if built.Round > rp.round {
			// update rp.round
			rp.round = built.Round
		}

		// we do accept quorum if it was built using
		// the peers we got registered as candidates
		if matchCandidates == true {
			rp.election = nil
			if rp.quorum == nil {
				rp.quorum = &RaftQuorum{}
				rp.quorum.State = candidateQuorumState
				rp.quorum.Member = false
				rp.quorum.Peers = built.Peers
				// QUODBG fmt.Println(rp.Name(), "QUO BUILT. NOT A MEMBER", rp.quorum.State, rp.quorum.Peers)
				return rp.handleQuorum()
			}
			// QUODBG fmt.Println(rp.Name(), "QUO BUILT. NOT A MEMBER", rp.quorum.State, rp.quorum.Peers)

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
				return rp.handleQuorum()
			}
			return RaftStatusOK
		}

		if rp.quorum != nil {
			rp.quorum = nil
			rp.election = nil
			return rp.handleQuorum()
		}
		return RaftStatusOK

	case etf.Atom("$leader_heartbeat"):
		heartbeat := &messageRaftLeaderHeartbeat{}
		if err := etf.TermIntoStruct(m.Command, &heartbeat); err != nil {
			return lib.ErrUnsupportedRequest
		}

		if rp.options.ID != heartbeat.ID {
			return RaftStatusOK
		}

		c := rp.quorumCandidates.GetOnline(m.Pid)
		if c == nil {
			// HRTDBG fmt.Println(rp.Self(), "HRT from unknown/offline peer", m.Pid)
			rp.quorumCandidates.Set(rp, m.Pid)
			return RaftStatusOK
		}
		// HRTDBG fmt.Println(rp.Self(), "HRT from", m.Pid, "serial", c.serial)
		c.heartbeat = time.Now().Unix()
		c.serial = heartbeat.Serial
		c.failures = 0
		return RaftStatusOK

	case etf.Atom("$quorum_leave"):
		leave := &messageRaftQuorumLeave{}
		if err := etf.TermIntoStruct(m.Command, &leave); err != nil {
			return lib.ErrUnsupportedRequest
		}
		if rp.quorum == nil {
			return RaftStatusOK
		}

		if rp.options.ID != leave.ID {
			return RaftStatusOK
		}

		// check if it came from the quorum member
		if rp.isQuorumMember(m.Pid) == false {
			return RaftStatusOK
		}

		// QUODBG fmt.Println(rp.Self(), "QUO got leave from", m.Pid, "due to", leave.DueToPid)
		rp.quorumCandidates.SetOffline(rp, leave.DueToPid)

		member := rp.quorum.Member
		rp.quorum = nil
		rp.handleQuorum()
		// only quorum member can restart quorum building if some of the member has left
		if member == true {
			rp.quorumChangeStart(false)
		}
		return RaftStatusOK

	case etf.Atom("$leader_vote"):
		vote := &messageRaftLeaderVote{}
		if err := etf.TermIntoStruct(m.Command, &vote); err != nil {
			return lib.ErrUnsupportedRequest
		}

		if rp.options.ID != vote.ID {
			lib.Warning("[%s] ignore 'leader vote' message being not a member of the given raft cluster (from %s)", rp.Self(), m.Pid)
			return RaftStatusOK
		}

		if rp.quorum == nil {
			rp.election = nil
			// no quorum
			// LDRDBG fmt.Println(rp.Self(), "LDR NO QUO ignore vote from", m.Pid, "round", vote.Round, "for", vote.Leader)
			// Seems we have received leader_vote before the quorum_built message.
			// Ignore this vote but update its round value to start a new leader election.
			// Otherwise, the new election will be started with the same round value but without
			// votes, which have been ignored before the quorum was built.
			if vote.Round > rp.round {
				rp.round = vote.Round
			}
			return RaftStatusOK
		}

		if rp.quorum.State != RaftQuorumState(vote.State) {
			// vote within another quorum. seems the quorum has been changed during this election.
			// ignore it
			// LDRDBG fmt.Println(rp.Self(), "LDR ignore vote from", m.Pid, "with another quorum", vote.State, "current quorum", rp.quorum.State)
			if vote.Round > rp.round {
				rp.round = vote.Round
			}
			return RaftStatusOK
		}
		if rp.election != nil && rp.election.round > vote.Round {
			// ignore it. current election round is greater
			// LDRDBG fmt.Println(rp.Self(), "LDR ignore vote from", m.Pid, "with round", vote.Round, "current election round", rp.election.round)
			return RaftStatusOK
		}
		if rp.round > vote.Round {
			// newbie is trying to start a new election :)
			// LDRDBG fmt.Println(rp.Self(), "LDR ignore vote from newbie", m.Pid, "with round", vote.Round, "current round", rp.round)
			return RaftStatusOK
		}

		// check if m.Pid is belongs to the quorum
		belongs := false
		for _, pid := range rp.quorum.Peers {
			if pid == m.Pid {
				belongs = true
				break
			}
		}

		if belongs == false {
			// there might be a case if we got vote message before the quorum_built
			lib.Warning("[%s] got ignore from the peer, which doesn't belong to the quorum %s", rp.Self(), m.Pid)
			if vote.Round > rp.round {
				rp.round = vote.Round
			}
			return RaftStatusOK
		}

		// start new election
		new_election := false
		switch {
		case rp.election == nil:
			new_election = true
		case rp.election != nil:
			// TODO case with existing leader whithin this quorum. if some of the quorum member
			// got leader heartbeat timeout it starts new election but this process has no problem
			// with the leader.
			if vote.Round > rp.election.round {
				// overwrite election if it has greater round number
				rp.election.cancel()
				new_election = true
			}
		}
		if new_election {
			// LDRDBG fmt.Println(rp.Self(), "LDR accept election from", m.Pid, "round", vote.Round, " with vote for:", vote.Leader)
			rp.election = &leaderElection{
				votes:   make(map[etf.Pid]etf.Pid),
				results: make(map[etf.Pid]bool),
				round:   vote.Round,
			}
			rp.election.cancel = rp.CastAfter(rp.Self, messageRaftElectionClean{round: vote.Round}, cleanLeaderVoteTimeout)
			rp.handleElectionVote()
		}

		if _, exist := rp.election.votes[m.Pid]; exist {
			lib.Warning("[%s] ignore duplicate vote for %s from %s during %d round", rp.Self(),
				vote.Leader, m.Pid, vote.Round)
			return RaftStatusOK
		}

		rp.election.votes[m.Pid] = vote.Leader
		// LDRDBG fmt.Println(rp.Self(), "LDR got vote from", m.Pid, "for", vote.Leader, "round", vote.Round, "quorum", vote.State)
		if len(rp.quorum.Peers) != len(rp.election.votes) {
			// make sure if we got all votes
			return RaftStatusOK
		}
		if len(rp.election.votes) != len(rp.quorum.Peers) {
			// waiting for all votes from the quorum members)
			return RaftStatusOK
		}

		// got all votes. count them to get the quorum leader
		countVotes := make(map[etf.Pid]int)
		for _, vote_for := range rp.election.votes {
			c, _ := countVotes[vote_for]
			countVotes[vote_for] = c + 1
		}
		leaderPid := etf.Pid{}
		leaderVoted := 0
		leaderSplit := false
		for leader, voted := range countVotes {
			if leaderVoted == voted {
				leaderSplit = true
				continue
			}
			if leaderVoted < voted {
				leaderVoted = voted
				leaderPid = leader
				leaderSplit = false
			}
		}
		// LDRDBG fmt.Println(rp.Self(), "LDR got all votes. round", vote.Round, "quorum", vote.State)
		if leaderSplit {
			// LDRDBG fmt.Println(rp.Self(), "LDR got split voices. round", vote.Round, "quorum", vote.State)
			// got more than one leader
			// start new leader election with round++
			rp.handleElectionStart(vote.Round + 1)
			return RaftStatusOK
		}

		noLeader := etf.Pid{}
		if rp.election.leader == noLeader {
			rp.election.leader = leaderPid
			rp.election.voted = leaderVoted
		} else {
			if rp.election.leader != leaderPid || rp.election.voted != leaderVoted {
				// our result defers from the others which we already received
				// start new leader election with round++
				lib.Warning("[%s] got different result from %s. cheating detected", rp.Self(), m.Pid)
				rp.handleElectionStart(vote.Round + 1)
				return RaftStatusOK
			}
		}

		// LDRDBG fmt.Println(rp.Self(), "LDR election done. round", rp.election.round, "Leader", leaderPid, "with", leaderVoted, "voices", "quorum", vote.State)
		rp.election.results[rp.Self()] = true

		// send to all quorum members our choice
		elected := etf.Tuple{
			etf.Atom("$leader_elected"),
			rp.Self(),
			etf.Tuple{
				rp.options.ID,
				leaderPid,
				leaderVoted,
				rp.election.round,
			},
		}
		for _, pid := range rp.quorum.Peers {
			if pid == rp.Self() {
				continue
			}
			rp.Cast(pid, elected)
			// LDRDBG fmt.Println(rp.Self(), "LDR elected", leaderPid, "sent result to", pid, "wait the others")
		}

		if len(rp.election.votes) != len(rp.election.results) {
			// we should wait for result from all the election members
			return RaftStatusOK
		}

		// leader has been elected
		// LDRDBG fmt.Println(rp.Self(), "LDR finished. leader", rp.election.leader, "round", rp.election.round, "quorum", rp.quorum.State)
		rp.round = rp.election.round
		rp.election.cancel()
		if rp.leader != rp.election.leader {
			rp.leader = rp.election.leader
			l := rp.Leader()
			rp.election = nil
			return rp.behavior.HandleLeader(rp, l)
		}
		rp.election = nil
		return RaftStatusOK

	case etf.Atom("$leader_elected"):
		elected := &messageRaftLeaderElected{}
		if err := etf.TermIntoStruct(m.Command, &elected); err != nil {
			return lib.ErrUnsupportedRequest
		}

		if rp.options.ID != elected.ID {
			lib.Warning("[%s] ignore 'leader elected' message being not a member of the given raft cluster (from %s)", rp.Self(), m.Pid)
			return RaftStatusOK
		}

		if rp.quorum == nil {
			rp.election = nil
			// no quorum
			// LDRDBG fmt.Println(rp.Self, "LDR NO QUO ignore election result", elected, "from", m.Pid)
			return RaftStatusOK
		}

		if rp.election == nil {
			lib.Warning("[%s] ignore election result from %s. no election on this peer", rp.Self(), m.Pid)
			return RaftStatusOK
		}

		if elected.Round != rp.election.round {
			// round value must be the same. seemd another election is started
			lib.Warning("[%s] ignore election result from %s with another round value %d (current election round %d)", rp.Self(), m.Pid, elected.Round, rp.election.round)
			if elected.Round > rp.round {
				// update round value to the greatest one
				rp.round = elected.Round
			}
			return RaftStatusOK
		}

		noLeader := etf.Pid{}
		if rp.election.leader == noLeader {
			rp.election.leader = elected.Leader
			rp.election.voted = elected.Voted
		} else {
			if rp.election.leader != elected.Leader || rp.election.voted != elected.Voted {
				// elected leader must be the same in all election results
				lib.Warning("[%s] ignore election result from %s with different leader which must be the same", rp.Self(), m.Pid)
				return RaftStatusOK
			}
		}

		if _, exist := rp.election.results[m.Pid]; exist {
			// duplicate
			lib.Warning("[%s] ignore duplicate election result from %s during %d round", rp.Self(),
				m.Pid, elected.Round)
			return RaftStatusOK
		}

		if _, exist := rp.election.votes[m.Pid]; exist == false {
			// Got election result before the vote from m.Pid
			// Check if m.Pid belongs to the quorum
			if rp.election.round > rp.round {
				rp.round = rp.election.round
			}
			belongs := false
			for _, pid := range rp.quorum.Peers {
				if pid == m.Pid {
					belongs = true
					break
				}
			}
			if belongs == false {
				// got from unknown peer
				lib.Warning("[%s] ignore election result from %s which doesn't belong this quorum", rp.Self(), m.Pid)
				return RaftStatusOK
			}

			// keep it and wait for the vote from this peer
			rp.election.results[m.Pid] = true
			return RaftStatusOK
		}
		rp.election.results[m.Pid] = true

		if len(rp.quorum.Peers) != len(rp.election.votes) {
			// make sure if we got all votes
			return RaftStatusOK
		}

		if len(rp.election.votes) != len(rp.election.results) {
			// we should wait for result from all the election members
			return RaftStatusOK
		}

		// leader has been elected
		// LDRDBG fmt.Println(rp.Self(), "LDR finished. leader", rp.election.leader, "round", rp.election.round, "quorum", rp.quorum.State)
		rp.election.cancel() // cancel timer
		rp.round = rp.election.round
		if rp.leader != rp.election.leader {
			rp.leader = rp.election.leader
			rp.election = nil
			l := rp.Leader()
			return rp.behavior.HandleLeader(rp, l)
		}
		rp.election = nil
		return RaftStatusOK

	case etf.Atom("$request_get"):
		requestGet := &messageRaftRequestGet{}
		if err := etf.TermIntoStruct(m.Command, &requestGet); err != nil {
			return lib.ErrUnsupportedRequest
		}

		if rp.options.ID != requestGet.ID {
			lib.Warning("[%s] got 'get' request being not a member of the given raft cluster (from %s)", rp.Self(), m.Pid)
			return RaftStatusOK
		}

		if rp.quorum == nil {
			// no quorum
			return RaftStatusOK
		}

		if rp.quorum.Member == false {
			// not a quorum member. couldn't handle this request
			lib.Warning("[%s] got 'get' request being not a member of the quorum (from %s)", rp.Self(), m.Pid)
			return RaftStatusOK
		}
		//fmt.Println(rp.Self(), "GET request", requestGet.Ref, "from", m.Pid, "serial", requestGet.Serial)

		key, value, status := rp.behavior.HandleGet(rp, requestGet.Serial)
		if status != RaftStatusOK {
			// do nothing
			return status
		}
		if value == nil {
			// not found.
			if m.Pid != requestGet.Origin {
				// its already forwarded request. just ignore it
				return RaftStatusOK
			}

			// forward this request to another qourum member
			forwardGet := etf.Tuple{
				etf.Atom("$request_get"),
				rp.Self(),
				etf.Tuple{
					requestGet.ID,
					requestGet.Ref,
					requestGet.Origin,
					requestGet.Serial,
				},
			}

			// get random quorum member excluding m.Pid and requestGet.Origin
			peers := []etf.Pid{}
			for _, pid := range rp.quorum.Peers {
				if pid == m.Pid {
					continue
				}
				if pid == requestGet.Origin {
					continue
				}
				if pid == rp.Self() {
					continue
				}
				peers = append(peers, pid)
			}

			if len(peers) == 0 {
				return RaftStatusOK
			}

			n := 0
			if len(peers) > 1 {
				n = rand.Intn(len(peers) - 1)
			}
			peer := peers[n]
			//fmt.Println(rp.Self(), "GET forward", requestGet.Ref, "to", peer, "serial", requestGet.Serial)
			rp.Cast(peer, forwardGet)
			return RaftStatusOK
		}

		requestReply := etf.Tuple{
			etf.Atom("$request_reply"),
			rp.Self(),
			etf.Tuple{
				requestGet.ID,
				requestGet.Ref,
				requestGet.Serial,
				key,
				value,
			},
		}
		rp.Cast(requestGet.Origin, requestReply)

		// update serial of this peer
		if c := rp.quorumCandidates.GetOnline(requestGet.Origin); c != nil {
			if c.serial < requestGet.Serial {
				c.serial = requestGet.Serial
			}
		} else {
			rp.quorumCandidates.Set(rp, requestGet.Origin)
		}
		return RaftStatusOK

	case etf.Atom("$request_reply"):
		requestReply := &messageRaftRequestReply{}
		if err := etf.TermIntoStruct(m.Command, &requestReply); err != nil {
			return lib.ErrUnsupportedRequest
		}

		if rp.options.ID != requestReply.ID {
			lib.Warning("[%s] got 'reply' being not a member of the given raft cluster (from %s)", rp.Self(), m.Pid)
			return RaftStatusOK
		}
		cancel, exist := rp.requests[requestReply.Ref]
		if exist == false {
			// might be timed out already. do nothing
			return RaftStatusOK
		}
		// cancel timer
		cancel()
		if rp.options.Serial < requestReply.Serial {
			rp.options.Serial = requestReply.Serial
		}
		// call HandleSerial
		return rp.behavior.HandleSerial(rp, requestReply.Ref, requestReply.Serial,
			requestReply.Key, requestReply.Value)

	case etf.Atom("$request_append"):
		requestAppend := &messageRaftRequestAppend{}
		if err := etf.TermIntoStruct(m.Command, &requestAppend); err != nil {
			return lib.ErrUnsupportedRequest
		}

		if rp.options.ID != requestAppend.ID {
			lib.Warning("[%s] got 'append' request being not a member of the given raft cluster (from %s)", rp.Self(), m.Pid)
			return RaftStatusOK
		}

		if rp.quorum == nil {
			// no quorum. ignore it
			return RaftStatusOK
		}

		//
		// There are 3 options:
		//

		// 1) This process is a leader -> handleAppendLeader()
		//   a) increment serial. send this request to all quorum members (except the origin peer)
		//   b) wait for the request_append_ready from the quorum peers
		//   c) call the callback HandleAppend
		//   d) send request_append_commit(serial) to all quorum members (including the origin peer)
		if rp.leader == rp.Self() {
			return rp.handleAppendLeader(m.Pid, requestAppend)
		}

		// 2) This process is not a leader, is a quorum member, and request has
		//    received from the leader -> handleAppendQuorum()
		//   a) accept this request and reply with request_append_ready
		//   b) wait for the request_append_commit
		//   c) call the callback HandleAppend
		//   d) send request_append to the peers that are not in the quorum
		if rp.quorum.Member == true && m.Pid == rp.leader {
			return rp.handleAppendQuorum(requestAppend)
		}

		// 3) This process neither a leader or a quorum member.
		// Or this process is a quorum member but request has received not from
		// the leader of this quorum.
		// It also could happened if quorum has changed during the delivering this request.

		// Forward this request to the quorum member (if this process not a quorum member)
		// or to the leader (if this process is a quorum member)

		forwardAppend := etf.Tuple{
			etf.Atom("$request_append"),
			rp.Self(),
			etf.Tuple{
				requestAppend.ID,
				requestAppend.Ref,
				requestAppend.Origin,
				requestAppend.Key,
				requestAppend.Value,
				requestAppend.Deadline,
			},
		}

		if rp.quorum.Member == true {
			// DBGAPN fmt.Println(rp.Self(), "DPGAPN forward $request_append", requestAppend.Ref, "to the leader", rp.leader)
			noLeader := etf.Pid{}
			if rp.leader == noLeader {
				// no leader in this quorum yet. ignore this request
				return RaftStatusOK
			}
			// This request has received not from the quorum leader.
			// Forward this request to the leader
			rp.Cast(rp.leader, forwardAppend)
			return RaftStatusOK
		}

		// exclude requestAppend.Origin and m.Pid
		peers := []etf.Pid{}
		for _, pid := range rp.quorum.Peers {
			if pid == m.Pid {
				continue
			}
			if pid == requestAppend.Origin {
				continue
			}
			peers = append(peers, pid)
		}
		n := rand.Intn(len(peers) - 1)
		peer := peers[n]
		// DBGAPN fmt.Println(rp.Self(), "DPGAPN forward $request_append", requestAppend.Ref, "to the quorum member", peer)
		rp.Cast(peer, forwardAppend)
		return RaftStatusOK

	case etf.Atom("$request_append_ready"):
		appendReady := &messageRaftAppendReady{}
		if err := etf.TermIntoStruct(m.Command, &appendReady); err != nil {
			return lib.ErrUnsupportedRequest
		}

		if rp.options.ID != appendReady.ID {
			lib.Warning("[%s] got 'append_ready' message being not a member of the given raft cluster (from %s)", rp.Self(), m.Pid)
			return RaftStatusOK
		}

		if rp.quorum == nil {
			// no quorum. ignore it
			return RaftStatusOK
		}

		requestAppend, exist := rp.requestsAppend[appendReady.Key]
		if exist == false {
			// there might be timeout happened. ignore this message
			return RaftStatusOK
		}

		if requestAppend.ref != appendReady.Ref {
			// there might be timeout happened for the previous append request for this key
			// and another append request arrived during previous append request handling
			return RaftStatusOK
		}

		if rp.leader != rp.Self() {
			// i'm not a leader. seems leader election happened during this request handling
			requestAppend.cancel()
			delete(rp.requestsAppend, appendReady.Key)
			return RaftStatusOK
		}
		requestAppend.peers[m.Pid] = true
		commit := true
		for _, confirmed := range requestAppend.peers {
			if confirmed {
				continue
			}
			commit = false
			break
		}

		if commit == false {
			return RaftStatusOK
		}

		// received confirmations from all the peers are involved to this append handling.
		// call HandleAppend
		status := rp.behavior.HandleAppend(rp, requestAppend.ref, rp.options.Serial+1,
			appendReady.Key, requestAppend.value)
		switch status {
		case RaftStatusOK:
			rp.options.Serial++
			// sent them $request_append_commit including the origin
			request := etf.Tuple{
				etf.Atom("$request_append_commit"),
				rp.Self(),
				etf.Tuple{
					rp.options.ID,
					requestAppend.ref,
					appendReady.Key,
					rp.options.Serial,
					requestAppend.from,
				},
			}
			for pid := range requestAppend.peers {
				if pid == rp.Self() {
					continue
				}
				rp.Cast(pid, request)
				// DBGAPN fmt.Println(rp.Self(), "DBGAPN sent append_commit to", pid, "with serial", rp.options.Serial)
				if c := rp.quorumCandidates.GetOnline(pid); c != nil {
					if c.serial < rp.options.Serial {
						c.serial = rp.options.Serial
					}
				}
			}
			requestAppend.cancel()
			delete(rp.requestsAppend, appendReady.Key)
			if requestAppend.from == rp.Self() {
				rp.handleBroadcastCommit(appendReady.Key, requestAppend, rp.options.Serial)
			}
			if len(rp.requestsAppendQueue) == 0 {
				return RaftStatusOK
			}

			// handle queued append request
			handled := 0
			for i := range rp.requestsAppendQueue {
				handled = i
				queued := rp.requestsAppendQueue[i]
				if queued.request.Deadline < time.Now().UnixMilli() {
					// expired request
					lib.Warning("[%s] append request %s is expired", rp.Self(), queued.request.Ref)
					continue
				}
				rp.handleAppendLeader(queued.from, queued.request)
				break
			}
			rp.requestsAppendQueue = rp.requestsAppendQueue[handled+1:]
			if len(rp.requestsAppendQueue) == 0 {
				rp.requestsAppendQueue = nil
			}
			return RaftStatusOK

		case RaftStatusDiscard:
			requestAppend.cancel()
			delete(rp.requestsAppend, appendReady.Key)
			return RaftStatusOK
		}

		return status

	case etf.Atom("$request_append_commit"):
		appendCommit := &messageRaftAppendCommit{}
		if err := etf.TermIntoStruct(m.Command, &appendCommit); err != nil {
			return lib.ErrUnsupportedRequest
		}

		if rp.options.ID != appendCommit.ID {
			lib.Warning("[%s] got 'append_commit' message being not a member of the given raft cluster (from %s)", rp.Self(), m.Pid)
			return RaftStatusOK
		}

		requestAppend, exist := rp.requestsAppend[appendCommit.Key]
		if exist == false {
			// seems timeout happened and this request was cleaned up
			return RaftStatusOK
		}
		requestAppend.cancel()
		delete(rp.requestsAppend, appendCommit.Key)

		if rp.options.Serial >= appendCommit.Serial {
			lib.Warning("[%s] got append commit with serial (%d) greater or equal we have (%d). fork happened. stopping this process", rp.Self(), appendCommit.Serial, rp.options.Serial)
			return fmt.Errorf("raft fork happened")
		}

		rp.options.Serial = appendCommit.Serial
		status := rp.behavior.HandleAppend(rp, requestAppend.ref, appendCommit.Serial,
			appendCommit.Key, requestAppend.value)
		if status == RaftStatusDiscard {
			lib.Warning("[%s] RaftStatusDiscard can be used by a leader only", rp.Self())
			status = RaftStatusOK
		}
		if appendCommit.Broadcast != rp.Self() {
			return status
		}

		rp.handleBroadcastCommit(appendCommit.Key, requestAppend, appendCommit.Serial)
		return status

	case etf.Atom("$request_append_broadcast"):
		broadcast := &messageRaftAppendBroadcast{}
		if err := etf.TermIntoStruct(m.Command, &broadcast); err != nil {
			return lib.ErrUnsupportedRequest
		}

		if rp.options.ID != broadcast.ID {
			lib.Warning("[%s] got 'append_broadcast' message being not a member of the given raft cluster (from %s)", rp.Self(), m.Pid)
			return RaftStatusOK
		}

		rp.options.Serial = broadcast.Serial
		return rp.behavior.HandleAppend(rp, broadcast.Ref, broadcast.Serial,
			broadcast.Key, broadcast.Value)

	}

	return lib.ErrUnsupportedRequest
}

func (rp *RaftProcess) handleElectionStart(round int) {
	if rp.quorum == nil {
		// no quorum. can't start election
		return
	}
	if rp.quorum.Member == false {
		// not a quorum member
		return
	}
	if rp.election != nil {
		if rp.election.round >= round {
			// already in progress
			return
		}
		rp.election.cancel()
	}
	if rp.round > round {
		round = rp.round
	}
	// LDRDBG fmt.Println(rp.Self(), "LDR start. round", round, "Q", rp.quorum.State)
	rp.election = &leaderElection{
		votes:   make(map[etf.Pid]etf.Pid),
		results: make(map[etf.Pid]bool),
		round:   round,
	}
	rp.handleElectionVote()
	cancel := rp.CastAfter(rp.Self, messageRaftElectionClean{round: round}, cleanLeaderVoteTimeout)
	rp.election.cancel = cancel
}

func (rp *RaftProcess) handleElectionVote() {
	if rp.quorum == nil || rp.election == nil {
		return
	}

	mapPeers := make(map[etf.Pid]bool)
	for _, p := range rp.quorum.Peers {
		mapPeers[p] = true
	}

	voted_for := etf.Pid{}
	c := rp.quorumCandidates.List() // ordered by serial in desk order
	for _, pid := range c {
		// check if this candidate is a member of quorum
		if _, exist := mapPeers[pid]; exist == false {
			continue
		}
		// get the first member since it has biggest serial
		voted_for = pid
		break
	}

	// LDRDBG fmt.Println(rp.Self(), "LDR voted for:", voted_for, "quorum", rp.quorum.State)
	leaderVote := etf.Tuple{
		etf.Atom("$leader_vote"),
		rp.Self(),
		etf.Tuple{
			rp.options.ID,
			int(rp.quorum.State),
			voted_for,
			rp.election.round,
		},
	}
	for _, pid := range rp.quorum.Peers {
		if pid == rp.Self() {
			continue
		}
		// LDRDBG fmt.Println(rp.Self(), "LDR sent vote for", voted_for, "to", pid, "round", rp.election.round, "quorum", rp.quorum.State)
		rp.Cast(pid, leaderVote)
	}
	rp.election.votes[rp.Self()] = voted_for
}

func (rp *RaftProcess) handleBroadcastCommit(key string, request *requestAppend, serial uint64) {
	// DBGAPN fmt.Println(rp.Self(), "broadcasting", request.ref)
	// the origin process is in charge of broadcasting this result among
	// the peers who aren't quorum members.
	commit := etf.Tuple{
		etf.Atom("$request_append_broadcast"),
		rp.Self(),
		etf.Tuple{
			rp.options.ID,
			request.ref,
			serial,
			key,
			request.value,
		},
	}
	allPeers := rp.quorumCandidates.List()
	for _, pid := range allPeers {
		if _, exist := request.peers[pid]; exist {
			continue
		}
		if pid == rp.Self() {
			continue
		}
		rp.Cast(pid, commit)
		// DBGAPN fmt.Println(rp.Self(), "DBGAPN sent $request_append_broadcast", request.ref, "to", pid)
		c := rp.quorumCandidates.GetOnline(pid)
		if c != nil && c.serial < serial {
			c.serial = serial
		}
	}
}

func (rp *RaftProcess) handleAppendLeader(from etf.Pid, request *messageRaftRequestAppend) RaftStatus {
	// DBGAPN fmt.Println(rp.Self(), "DBGAPN handle append", request.Ref, "on leader.", request.Key, request.Value)
	if _, exist := rp.requestsAppend[request.Key]; exist {
		// another append request with this key is still in progress. append to the queue
		queued := requestAppendQueued{
			from:    from,
			request: request,
		}
		rp.requestsAppendQueue = append(rp.requestsAppendQueue, queued)
		lq := len(rp.requestsAppendQueue)
		if lq > 10 {
			lib.Warning("[%s] append request queue is getting long. queued request %d", rp.Self(), lq)
		}
		return RaftStatusOK
	}
	now := time.Now().UnixMilli()
	if now >= request.Deadline {
		// deadline has been passed. ignore this request
		return RaftStatusOK
	}

	sendRequestAppend := etf.Tuple{
		etf.Atom("$request_append"),
		rp.Self(),
		etf.Tuple{
			rp.options.ID,
			request.Ref,
			request.Origin,
			request.Key,
			request.Value,
			request.Deadline,
		},
	}

	peers := make(map[etf.Pid]bool)
	for _, pid := range rp.quorum.Peers {
		if pid == rp.Self() {
			continue
		}
		peers[pid] = false
		if pid == request.Origin {
			peers[pid] = true // do not wait append_ready for the Origin
			continue
		}
		rp.Cast(pid, sendRequestAppend)
	}

	// if 'from' is not a quorum member the leader is in charge of broadcasting
	if _, exist := peers[from]; exist == false {
		from = rp.Self()
	}

	after := time.Duration(request.Deadline-now) * time.Millisecond
	clean := messageRaftAppendClean{key: request.Key, ref: request.Ref}
	cancel := rp.CastAfter(rp.Self(), clean, after)
	requestAppend := &requestAppend{
		ref:    request.Ref,
		from:   from,
		origin: request.Origin,
		value:  request.Value,
		peers:  peers,
		cancel: cancel,
	}
	rp.requestsAppend[request.Key] = requestAppend

	return RaftStatusOK
}

func (rp *RaftProcess) handleAppendQuorum(request *messageRaftRequestAppend) RaftStatus {
	// DBGAPN fmt.Println(rp.Self(), "DBGAPN handle append", request.Ref, "on a quorum member.", request.Key, request.Value)
	if r, exist := rp.requestsAppend[request.Key]; exist {
		r.cancel()
		delete(rp.requestsAppend, request.Key)
	}

	ready := etf.Tuple{
		etf.Atom("$request_append_ready"),
		rp.Self(),
		etf.Tuple{
			rp.options.ID,
			request.Ref,
			request.Key,
		},
	}
	rp.Cast(rp.leader, ready)
	clean := messageRaftAppendClean{key: request.Key, ref: request.Ref}
	after := time.Duration(DefaultRaftAppendTimeout) * time.Second
	if d := time.Duration(request.Deadline-time.Now().UnixMilli()) * time.Millisecond; d > after {
		after = d
	}
	cancel := rp.CastAfter(rp.Self, clean, after)

	peers := make(map[etf.Pid]bool)
	for _, pid := range rp.quorum.Peers {
		peers[pid] = true
	}

	requestAppend := &requestAppend{
		ref:    request.Ref,
		origin: request.Origin,
		value:  request.Value,
		peers:  peers,
		cancel: cancel,
	}
	rp.requestsAppend[request.Key] = requestAppend
	return RaftStatusOK
}

func (rp *RaftProcess) quorumChangeStart(nextAttempt bool) {
	if rp.quorumChangeDefer == false {
		if nextAttempt {
			// increase timeout for the next attempt to build a new quorum
			rp.quorumChangeAttempt++
		} else {
			rp.quorumChangeAttempt = 1
		}
		maxTime := rp.quorumChangeAttempt * quorumChangeDeferMaxTime
		after := time.Duration(50+rand.Intn(maxTime)) * time.Millisecond
		rp.CastAfter(rp.Self(), messageRaftQuorumChange{}, after)
		rp.quorumChangeDefer = true
	}
}

func (rp *RaftProcess) quorumChange() RaftStatus {
	l := len(rp.quorumCandidates.List())

	candidateRaftQuorumState := RaftQuorumState3
	switch {
	case l > 9:
		if rp.quorum != nil && rp.quorum.State == RaftQuorumState11 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState11
		l = 10 // to create quorum of 11 we need 10 candidates + itself.

	case l > 7:
		if rp.quorum != nil && rp.quorum.State == RaftQuorumState9 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState9
		l = 8 // quorum of 9 => 8 candidates + itself
	case l > 5:
		if rp.quorum != nil && rp.quorum.State == RaftQuorumState7 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState7
		l = 6 // quorum of 7 => 6 candidates + itself
	case l > 3:
		if rp.quorum != nil && rp.quorum.State == RaftQuorumState5 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState5
		l = 4 // quorum of 5 => 4 candidates + itself
	case l > 1:
		if rp.quorum != nil && rp.quorum.State == RaftQuorumState3 {
			// do nothing
			return RaftStatusOK
		}
		candidateRaftQuorumState = RaftQuorumState3
		l = 2 // quorum of 3 => 2 candidates + itself
	default:
		// not enougth candidates to create a quorum
		if rp.quorum != nil {
			rp.quorum = nil
			return rp.handleQuorum()
		}
		// QUODBG fmt.Println(rp.Name(), "QUO VOTE. NOT ENO CAND", rp.quorumCandidates.List())

		// try send cluster_join again to receive an updated peer list
		rp.CastAfter(rp.Self(), messageRaftClusterInit{}, 5*time.Second)
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
	// QUODBG fmt.Println(rp.Name(), "QUO VOTE INIT", candidateRaftQuorumState, quorumCandidates)

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
			// QUODBG fmt.Println(rp.Name(), "SEND VOTE to", pid, q.Peers)
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
		// QUODBG fmt.Println(rp.Name(), "SEND VOTE to origin", q.origin, q.Peers)
		rp.Cast(q.origin, quorumVote)
	}

	return allVoted
}

func (rp *RaftProcess) quorumVote(from etf.Pid, vote *messageRaftQuorumVote) RaftStatus {
	if vote.State != len(vote.Candidates) {
		lib.Warning("[%s] quorum state and number of candidates are mismatch", rp.Self())
		rp.quorumCandidates.SetOffline(rp, from)
		return RaftStatusOK
	}

	if c := rp.quorumCandidates.GetOnline(from); c == nil {
		// there is a race conditioned case when we received a vote before
		// the quorum_join_reply message. just ignore it. they will start
		// another round of quorum forming
		return RaftStatusOK
	} else {
		c.heartbeat = time.Now().Unix()
		c.failures = 0
	}
	candidatesRaftQuorumState := RaftQuorumState3
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
		rp.quorumCandidates.SetOffline(rp, from)
		return RaftStatusOK
	}

	// do not vote if requested quorum is less than existing one
	if rp.quorum != nil && candidatesRaftQuorumState <= rp.quorum.State {
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

		// QUODBG fmt.Println(rp.Name(), "SKIP VOTE from", from, candidatesRaftQuorumState, rp.quorum.State)
		built := etf.Tuple{
			etf.Atom("$quorum_built"),
			rp.Self(),
			etf.Tuple{
				rp.options.ID,
				int(rp.quorum.State),
				rp.round,
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
		// QUODBG fmt.Println(rp.Name(), "QUO VOTE (NEW)", from, vote)
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
		// QUODBG fmt.Println(rp.Name(), "QUO VOTE", from, vote)
	}

	// returns true if we got votes from all the peers whithin this quorum
	if rp.quorumSendVote(q) == true {
		//
		// Quorum built
		//
		// QUODBG fmt.Println(rp.Name(), "QUO BUILT", q.State, q.Peers)
		if rp.quorum == nil {
			rp.quorum = &RaftQuorum{}
		}
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
					rp.round,
					rp.quorum.Peers,
				},
			}
			rp.Cast(peer, built)

		}

		rp.handleElectionStart(rp.round + 1)
		return rp.handleQuorum()
	}

	return RaftStatusOK
}

func (rp *RaftProcess) clusterHeal() {
	for _, pid := range rp.quorumCandidates.ListOffline() {
		// c can't be nil here
		c := rp.quorumCandidates.Get(pid)
		if c.heartbeat == 0 {
			continue
		}
		diff := time.Now().Unix() - c.heartbeat
		switch {
		case diff < 0:
			// heartbeat was set in the future
			continue
		case diff > 300: // > 5 min
			rp.Join(pid)
			// the next attempt will be in an hour
			c.heartbeat = time.Now().Unix() + 3600
		}
	}
}

func (rp *RaftProcess) handleQuorum() RaftStatus {
	q := rp.Quorum()
	if status := rp.behavior.HandleQuorum(rp, q); status != RaftStatusOK {
		return status
	}

	noLeader := etf.Pid{}
	if rp.leader != noLeader {
		rp.leader = noLeader
		if status := rp.behavior.HandleLeader(rp, nil); status != RaftStatusOK {
			return status
		}
	}

	if q == nil || q.Member == false {
		return RaftStatusOK
	}

	if rp.election == nil {
		rp.handleElectionStart(rp.round + 1)
	}

	return RaftStatusOK
}

func (rp *RaftProcess) handleHeartbeat() {
	if rp.heartbeatCancel != nil {
		rp.heartbeatCancel()
		rp.heartbeatCancel = nil
	}

	defer func() {
		after := DefaultRaftHeartbeat * time.Second
		cancel := rp.CastAfter(rp.Self(), messageRaftHeartbeat{}, after)
		rp.heartbeatCancel = cancel
		rp.clusterHeal()
	}()

	if rp.quorum == nil || rp.quorum.Member == false {
		return
	}

	noLeader := etf.Pid{}
	if rp.leader == noLeader {
		// leader election is still in progress. do nothing atm.
		return
	}

	if rp.leader == rp.Self() {
		// send a heartbeat to all quorum members if this process is a leader of this quorum
		heartbeat := etf.Tuple{
			etf.Atom("$leader_heartbeat"),
			rp.Self(),
			etf.Tuple{
				rp.options.ID,
				rp.options.Serial,
			},
		}
		for _, pid := range rp.quorum.Peers {
			if pid == rp.Self() {
				continue
			}
			rp.Cast(pid, heartbeat)
		}
		return
	}

	// check leader's heartbeat
	c := rp.quorumCandidates.GetOnline(rp.leader)
	if c != nil {
		diff := time.Now().Unix() - c.heartbeat
		if c.heartbeat == 0 {
			diff = 0
		}

		if diff < DefaultRaftHeartbeat*3 {
			return
		}

		// long time no see heartbeats from the leader
		c.joined = false
		rp.quorumCandidates.SetOffline(rp, rp.leader)
	}

	// HRTDBG fmt.Println(rp.Self(), "HRT lost leader", rp.leader)
	leave := etf.Tuple{
		etf.Atom("$quorum_leave"),
		rp.Self(),
		etf.Tuple{
			rp.options.ID,
			rp.leader,
		},
	}

	// tell everyone in the raft cluster
	for _, peer := range rp.quorumCandidates.List() {
		rp.Cast(peer, leave)
	}
	rp.quorum = nil
	rp.handleQuorum()
	rp.quorumChangeStart(false)
}

func (rp *RaftProcess) isQuorumMember(pid etf.Pid) bool {
	if rp.quorum == nil {
		return false
	}
	for _, peer := range rp.quorum.Peers {
		if pid == peer {
			return true
		}
	}
	return false
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
			rp.quorumCandidates.SetOffline(rp, from)
			return false
		}
		duplicates[pid] = false

		c := rp.quorumCandidates.GetOnline(pid)
		if c == nil {
			candidatesMatch = false
			rp.quorumCandidates.Set(rp, pid)
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
		// QUODBG fmt.Println(rp.Name(), "QUO CAND MISMATCH", from, vote.Candidates)
		return false
	}

	if validSerial == false {
		lib.Warning("[%s] got vote from %s with invalid serial", rp.Name(), from)
		rp.quorumCandidates.SetOffline(rp, from)
		return false
	}

	if validFrom == false || validTo == false {
		lib.Warning("[%s] got vote from %s with invalid data", rp.Name(), from)
		rp.quorumCandidates.SetOffline(rp, from)
		return false
	}

	// mark as recv
	v, _ := q.votes[from]
	q.votes[from] = v | 2

	return true
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
		requests:         make(map[etf.Ref]CancelFunc),
		requestsAppend:   make(map[string]*requestAppend),
	}

	// do not inherit parent State
	raftProcess.State = nil
	options, err := behavior.InitRaft(raftProcess, args...)
	if err != nil {
		return err
	}

	raftProcess.options = options
	process.State = raftProcess

	process.Cast(process.Self(), messageRaftClusterInit{})
	//process.SetTrapExit(true)
	raftProcess.handleHeartbeat()
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
	case messageRaftClusterInit:
		if rp.quorum != nil {
			return ServerStatusOK
		}
		if len(rp.quorumVotes) > 0 {
			return ServerStatusOK
		}
		for _, peer := range rp.options.Peers {
			rp.Join(peer)
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

		if q != nil {
			// QUODBG fmt.Println(rp.Name(), "CLN VOTE", m.state, q.Peers)
			delete(rp.quorumVotes, m.state)
			for _, peer := range q.Peers {
				v, _ := q.votes[peer]
				if v&2 > 0 { // vote received
					continue
				}
				// no vote from this peer. there are two options
				// 1. this peer has switched to the other quorum building
				// 2. something wrong with this peer (raft process could be stuck).
				c := rp.quorumCandidates.GetOnline(peer)
				if c == nil {
					// already offline
					continue
				}
				c.failures++
				if c.failures > 10 {
					// QUODBG fmt.Println(rp.Self(), "too many failures with", peer)
					rp.quorumCandidates.SetOffline(rp, peer)
				}
			}
		}
		if len(rp.quorumVotes) == 0 {
			// make another attempt to build new quorum
			rp.quorumChangeStart(true)
		}
	case messageRaftQuorumChange:
		rp.quorumChangeDefer = false
		status = rp.quorumChange()

	case messageRaftRequestClean:
		delete(rp.requests, m.ref)
		status = rp.behavior.HandleCancel(rp, m.ref, "timeout")

	case messageRaftAppendClean:
		request, exist := rp.requestsAppend[m.key]
		if exist == false {
			// do nothing
			return ServerStatusOK
		}
		if request.ref != m.ref {
			return ServerStatusOK
		}
		if request.origin == rp.Self() {
			status = rp.behavior.HandleCancel(rp, request.ref, "timeout")
			break
		}
		delete(rp.requestsAppend, m.key)
		return ServerStatusOK
	case messageRaftElectionClean:
		if rp.quorum == nil {
			return ServerStatusOK
		}
		if rp.election == nil && rp.quorum.Member {
			// restart election
			rp.handleElectionStart(rp.round + 1)
			return ServerStatusOK
		}
		if m.round != rp.election.round {
			// new election round happened
			// LDRDBG fmt.Println(rp.Self(), "LDR clean election. skip. new election round", rp.election.round)
			return ServerStatusOK
		}
		// LDRDBG fmt.Println(rp.Self(), "LDR clean election. round", rp.election.round)
		rp.election = nil
		return ServerStatusOK

	case messageRaftHeartbeat:
		rp.handleHeartbeat()
		return ServerStatusOK

	default:
		if err := etf.TermIntoStruct(message, &mRaft); err != nil {
			status = rp.behavior.HandleRaftInfo(rp, message)
			break
		}
		if mRaft.Pid == process.Self() {
			lib.Warning("[%s] got raft command from itself %#v", process.Self(), mRaft)
			return ServerStatusOK
		}
		status = rp.handleRaftRequest(mRaft)
		if status == lib.ErrUnsupportedRequest {
			status = rp.behavior.HandleRaftCast(rp, message)
		}
	}

	switch status {
	case nil, RaftStatusOK:
		return ServerStatusOK
	case RaftStatusStop:
		return ServerStatusStop
	case lib.ErrUnsupportedRequest:
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
		can := rp.quorumCandidates.GetOnline(m.Pid)
		if can == nil {
			break
		}
		if can.monitor != m.Ref {
			status = rp.behavior.HandleRaftInfo(rp, message)
			break
		}
		rp.quorumCandidates.SetOffline(rp, m.Pid)
		if rp.quorum == nil {
			return ServerStatusOK
		}
		for _, peer := range rp.quorum.Peers {
			// check if this pid belongs to the quorum
			if peer != m.Pid {
				continue
			}

			// start to build new quorum
			// QUODBG fmt.Println(rp.Name(), "QUO PEER DOWN", m.Pid)
			rp.handleQuorum()
			rp.quorumChangeStart(false)
			break
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

// HandleQuorum
func (r *Raft) HandleQuorum(process *RaftProcess, quorum *RaftQuorum) RaftStatus {
	return RaftStatusOK
}

// HandleLeader
func (r *Raft) HandleLeader(process *RaftProcess, leader *RaftLeader) RaftStatus {
	return RaftStatusOK
}

// HandlePeer
func (r *Raft) HandlePeer(process *RaftProcess, peer etf.Pid, serial uint64) RaftStatus {
	return RaftStatusOK
}

// HandleSerial
func (r *Raft) HandleSerial(process *RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) RaftStatus {
	lib.Warning("HandleSerial: unhandled key-value message with ref %s and serial %d", ref, serial)
	return RaftStatusOK
}

// HandleCancel
func (r *Raft) HandleCancel(process *RaftProcess, ref etf.Ref, reason string) RaftStatus {
	lib.Warning("HandleCancel: unhandled cancel with ref %s and reason %q", ref, reason)
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
	return nil, lib.ErrUnsupportedRequest
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

func (qc *quorumCandidates) Set(rp *RaftProcess, peer etf.Pid) {
	c, exist := qc.candidates[peer]
	if exist == true {
		diff := time.Now().Unix() - c.heartbeat
		if diff > DefaultRaftHeartbeat {
			rp.Join(peer)
		}
		return
	}
	c = &candidate{
		heartbeat: time.Now().Unix(),
	}
	qc.candidates[peer] = c
	rp.Join(peer)
}

func (qc *quorumCandidates) SetOnline(rp *RaftProcess, peer etf.Pid, serial uint64) bool {
	c, exist := qc.candidates[peer]
	if exist == false {
		return false
	}
	mon := rp.MonitorProcess(peer)
	c.serial = serial
	c.monitor = mon
	c.joined = true
	c.heartbeat = time.Now().Unix()
	c.failures = 0
	return true
}

func (qc *quorumCandidates) SetOffline(rp *RaftProcess, peer etf.Pid) {
	c, exist := qc.candidates[peer]
	if exist == false {
		return
	}
	// QUODBG fmt.Println(rp.Self(), "peer", peer, "has left")
	emptyRef := etf.Ref{}
	if c.monitor != emptyRef {
		rp.DemonitorProcess(c.monitor)
		c.monitor = emptyRef
	}
	c.joined = false
	c.failures = 0
	c.heartbeat = time.Now().Unix()
	return
}

func (qc *quorumCandidates) GetOnline(peer etf.Pid) *candidate {
	c, exist := qc.candidates[peer]
	if exist && c.joined == false {
		return nil
	}
	return c
}
func (qc *quorumCandidates) Get(peer etf.Pid) *candidate {
	c, exist := qc.candidates[peer]
	if exist == false {
		return nil
	}
	return c
}

// List returns list of online peers
func (qc *quorumCandidates) List() []etf.Pid {
	type c struct {
		pid    etf.Pid
		serial uint64
	}
	list := []c{}
	for k, v := range qc.candidates {
		if v.joined == false {
			continue
		}
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

func (qc *quorumCandidates) ListOffline() []etf.Pid {
	list := []etf.Pid{}
	for pid, c := range qc.candidates {
		if c.joined == true {
			continue
		}
		list = append(list, pid)
	}
	return list
}
