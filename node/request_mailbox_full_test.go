package node

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/testing/check"
)

type logCapture struct {
	mu   sync.Mutex
	logs []string
}

func (c *logCapture) dolog(m gen.MessageLog, _ string) {
	c.mu.Lock()
	c.logs = append(c.logs, fmt.Sprintf(m.Format, m.Args...))
	c.mu.Unlock()
}

func (c *logCapture) has(prefix string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, l := range c.logs {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}

type fullRequester struct {
	name    string
	queue   lib.QueueMPSC
	reg     *requests
	full    error
	logs    *logCapture
	deliver func(ref gen.Ref) error
	expire  func(ref gen.Ref)
}

func fullRequesters() []fullRequester {
	var list []fullRequester

	plogs := &logCapture{}
	p := &process{state: int32(gen.ProcessStateInit)}
	p.mailbox.Main = lib.NewQueueLimitMPSC(1)
	p.mailbox.System = lib.NewQueueLimitMPSC(1)
	p.mailbox.Urgent = lib.NewQueueLimitMPSC(1)
	p.log = createLog(gen.LogLevelInfo, plogs.dolog)
	list = append(list, fullRequester{
		name:  "process",
		queue: p.mailbox.Main,
		reg:   requestsOf(&p.requests),
		full:  gen.ErrProcessMailboxFull,
		logs:  plogs,
		deliver: func(ref gen.Ref) error {
			err, _ := p.deliverResponse(gen.PID{ID: 9}, gen.MessageOptions{Ref: ref}, "answer", nil)
			return err
		},
		expire: p.expireRequest,
	})

	mlogs := &logCapture{}
	m := &meta{
		p:      &process{state: int32(gen.ProcessStateInit)},
		main:   lib.NewQueueLimitMPSC(1),
		system: lib.NewQueueLimitMPSC(1),
		log:    createLog(gen.LogLevelInfo, mlogs.dolog),
	}
	list = append(list, fullRequester{
		name:  "meta",
		queue: m.main,
		reg:   requestsOf(&m.requests),
		full:  gen.ErrMetaMailboxFull,
		logs:  mlogs,
		deliver: func(ref gen.Ref) error {
			return m.deliverResponse(gen.PID{ID: 9}, gen.MessageOptions{Ref: ref}, "answer", nil)
		},
		expire: m.expireRequest,
	})
	return list
}

func (r fullRequester) fill(t *testing.T) {
	t.Helper()
	if r.queue.Push(gen.TakeMailboxMessage()) == false {
		t.Fatal("unable to fill the mailbox")
	}
}

func (r fullRequester) arm(ref gen.Ref, after time.Duration) {
	pending := &pendingRequest{priority: gen.MessagePriorityNormal}
	r.reg.mu.Lock()
	pending.timer = time.AfterFunc(after, func() { r.expire(ref) })
	r.reg.pending[ref] = pending
	r.reg.mu.Unlock()
}

func (r fullRequester) registered(ref gen.Ref) bool {
	r.reg.mu.Lock()
	defer r.reg.mu.Unlock()
	return r.reg.pending[ref] != nil
}

func (r fullRequester) answers() []gen.MessageResponse {
	var list []gen.MessageResponse
	for {
		v, ok := r.queue.Pop()
		if ok == false {
			return list
		}
		qm := v.(*gen.MailboxMessage)
		if qm.Type == gen.MailboxMessageTypeResponse {
			list = append(list, qm.Message.(gen.MessageResponse))
		}
	}
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for cond() == false {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestRequestResponseIntoFullMailbox: a response that finds the requester's
// mailbox full is dropped and logged, the responder learns the mailbox is full,
// and the request stays pending until its deadline answers it with ErrTimeout.
func TestRequestResponseIntoFullMailbox(t *testing.T) {
	for _, r := range fullRequesters() {
		t.Run(r.name, func(t *testing.T) {
			ref := gen.Ref{ID: [3]uint64{1, 2, 3}}
			r.fill(t)
			r.arm(ref, 300*time.Millisecond)

			check.True(t, r.deliver(ref) == r.full)
			check.True(t, r.logs.has("unable to deliver response on request"))
			check.True(t, r.registered(ref))

			check.Equal(t, 0, len(r.answers()))
			var answers []gen.MessageResponse
			waitFor(t, 2*time.Second, func() bool {
				answers = append(answers, r.answers()...)
				return len(answers) > 0
			})
			time.Sleep(100 * time.Millisecond)
			answers = append(answers, r.answers()...)

			check.Equal(t, 1, len(answers))
			check.ErrorIs(t, answers[0].Error, gen.ErrTimeout)
			check.True(t, r.registered(ref) == false)
		})
	}
}

// TestRequestTimeoutIntoFullMailbox: a timeout that finds the requester's
// mailbox full is dropped and logged; nothing arrives once there is room.
func TestRequestTimeoutIntoFullMailbox(t *testing.T) {
	for _, r := range fullRequesters() {
		t.Run(r.name, func(t *testing.T) {
			ref := gen.Ref{ID: [3]uint64{4, 5, 6}}
			r.fill(t)
			r.arm(ref, 100*time.Millisecond)

			waitFor(t, 2*time.Second, func() bool { return r.logs.has("unable to deliver timeout on request") })
			check.True(t, r.registered(ref) == false)

			check.Equal(t, 0, len(r.answers()))
			time.Sleep(200 * time.Millisecond)
			check.Equal(t, 0, len(r.answers()))
		})
	}
}
