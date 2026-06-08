package distributed

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// t11receiver counts received messages and signals when target is reached
type t11receiver struct {
	act.Actor

	target   int64
	count    atomic.Int64
	done     chan struct{}
	mismatch atomic.Int64
}

var (
	t11done     chan struct{}
	t11target   int64
	t11count    atomic.Int64
	t11mismatch atomic.Int64
)

func factory_t11receiver() gen.ProcessBehavior {
	return &t11receiver{}
}

func (t *t11receiver) Init(args ...any) error {
	return nil
}

func (t *t11receiver) HandleMessage(from gen.PID, message any) error {
	s, ok := message.(string)
	if ok == false {
		t11mismatch.Add(1)
		return nil
	}
	// verify payload integrity: first 10 chars are the expected length
	expected := fmt.Sprintf("%010d", len(s))
	if len(s) < 10 || s[:10] != expected {
		t11mismatch.Add(1)
		return nil
	}

	n := t11count.Add(1)
	if n >= t11target {
		select {
		case t11done <- struct{}{}:
		default:
		}
	}
	return nil
}

// t11sender sends N messages of given size to remote pid
type t11sender struct {
	act.Actor
}

func factory_t11sender() gen.ProcessBehavior {
	return &t11sender{}
}

type t11sendJob struct {
	target  gen.PID
	size    int
	count   int
	noOrder bool
	done    chan error
}

func (t *t11sender) HandleMessage(from gen.PID, message any) error {
	job, ok := message.(*t11sendJob)
	if ok == false {
		return nil
	}

	if job.noOrder {
		t.SetKeepNetworkOrder(false)
	}

	for i := 0; i < job.count; i++ {
		// create payload with size prefix for integrity check
		s := lib.RandomString(job.size)
		prefix := fmt.Sprintf("%010d", len(s))
		s = prefix + s[10:]

		if err := t.Send(job.target, s); err != nil {
			job.done <- fmt.Errorf("send error at msg %d: %w", i, err)
			return nil
		}
	}
	job.done <- nil
	return nil
}

func TestT11FragmentationLoad(t *testing.T) {
	t.Run("OrderedConcurrent", testFragLoadOrderedConcurrent)
	t.Run("UnorderedConcurrent", testFragLoadUnorderedConcurrent)
	t.Run("MixedOrderConcurrent", testFragLoadMixedConcurrent)
}

// 10 senders, each sends 100 ordered large messages
func testFragLoadOrderedConcurrent(t *testing.T) {
	node1, node2, receiverPID := setupFragLoadNodes(t, "Ordered")
	defer node1.Stop()
	defer node2.Stop()

	numSenders := 10
	msgsPerSender := 100
	msgSize := 20000 // ~5 fragments each at FragmentSize=4096
	totalMsgs := numSenders * msgsPerSender

	t11done = make(chan struct{}, 1)
	t11target = int64(totalMsgs)
	t11count.Store(0)
	t11mismatch.Store(0)

	// spawn senders and fire jobs
	var wg sync.WaitGroup
	for i := 0; i < numSenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pid, err := node1.Spawn(factory_t11sender, gen.ProcessOptions{})
			if err != nil {
				t.Errorf("spawn sender: %v", err)
				return
			}
			job := &t11sendJob{
				target:  receiverPID,
				size:    msgSize,
				count:   msgsPerSender,
				noOrder: false,
				done:    make(chan error, 1),
			}
			node1.Send(pid, job)
			if err := <-job.done; err != nil {
				t.Errorf("sender error: %v", err)
			}
		}()
	}

	// wait for all sends to complete
	wg.Wait()

	// wait for all messages to be received
	select {
	case <-t11done:
	case <-time.After(30 * time.Second):
		t.Fatalf("timeout: received %d/%d messages", t11count.Load(), totalMsgs)
	}

	if m := t11mismatch.Load(); m > 0 {
		t.Fatalf("payload integrity errors: %d", m)
	}
	t.Logf("OK: %d messages delivered, %d fragments each (~%d total fragments)",
		totalMsgs, msgSize/984+1, totalMsgs*(msgSize/984+1))
}

// 10 senders, each sends 100 unordered large messages
func testFragLoadUnorderedConcurrent(t *testing.T) {
	node1, node2, receiverPID := setupFragLoadNodes(t, "Unordered")
	defer node1.Stop()
	defer node2.Stop()

	numSenders := 10
	msgsPerSender := 100
	msgSize := 5000
	totalMsgs := numSenders * msgsPerSender

	t11done = make(chan struct{}, 1)
	t11target = int64(totalMsgs)
	t11count.Store(0)
	t11mismatch.Store(0)

	var wg sync.WaitGroup
	for i := 0; i < numSenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pid, err := node1.Spawn(factory_t11sender, gen.ProcessOptions{})
			if err != nil {
				t.Errorf("spawn sender: %v", err)
				return
			}
			job := &t11sendJob{
				target:  receiverPID,
				size:    msgSize,
				count:   msgsPerSender,
				noOrder: true,
				done:    make(chan error, 1),
			}
			node1.Send(pid, job)
			if err := <-job.done; err != nil {
				t.Errorf("sender error: %v", err)
			}
		}()
	}

	wg.Wait()

	select {
	case <-t11done:
	case <-time.After(30 * time.Second):
		t.Fatalf("timeout: received %d/%d messages", t11count.Load(), totalMsgs)
	}

	if m := t11mismatch.Load(); m > 0 {
		t.Fatalf("payload integrity errors: %d", m)
	}
	t.Logf("OK: %d messages delivered (unordered, shared assembly)", totalMsgs)
}

// 10 senders: 5 ordered + 5 unordered, mixed sizes
func testFragLoadMixedConcurrent(t *testing.T) {
	node1, node2, receiverPID := setupFragLoadNodes(t, "Mixed")
	defer node1.Stop()
	defer node2.Stop()

	numSenders := 10
	msgsPerSender := 100
	totalMsgs := numSenders * msgsPerSender

	t11done = make(chan struct{}, 1)
	t11target = int64(totalMsgs)
	t11count.Store(0)
	t11mismatch.Store(0)

	var wg sync.WaitGroup
	for i := 0; i < numSenders; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			pid, err := node1.Spawn(factory_t11sender, gen.ProcessOptions{})
			if err != nil {
				t.Errorf("spawn sender: %v", err)
				return
			}
			// odd senders: unordered, larger messages
			// even senders: ordered, smaller messages
			noOrder := idx%2 == 1
			size := 12000
			if noOrder {
				size = 32000
			}
			job := &t11sendJob{
				target:  receiverPID,
				size:    size,
				count:   msgsPerSender,
				noOrder: noOrder,
				done:    make(chan error, 1),
			}
			node1.Send(pid, job)
			if err := <-job.done; err != nil {
				t.Errorf("sender error: %v", err)
			}
		}()
	}

	wg.Wait()

	select {
	case <-t11done:
	case <-time.After(30 * time.Second):
		t.Fatalf("timeout: received %d/%d messages", t11count.Load(), totalMsgs)
	}

	if m := t11mismatch.Load(); m > 0 {
		t.Fatalf("payload integrity errors: %d", m)
	}
	t.Logf("OK: %d messages delivered (mixed ordered/unordered)", totalMsgs)
}

func setupFragLoadNodes(t *testing.T, suffix string) (gen.Node, gen.Node, gen.PID) {
	t.Helper()

	uid := lib.RandomString(6)

	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "fragload"
	options1.Network.FragmentSize = 4096
	options1.Log.DefaultLogger.Disable = true
	node1, err := ergo.StartNode(gen.Atom(fmt.Sprintf("distT11n1%s%s@localhost", suffix, uid)), options1)
	if err != nil {
		t.Fatal(err)
	}

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "fragload"
	options2.Network.FragmentSize = 4096
	options2.Log.DefaultLogger.Disable = true
	node2, err := ergo.StartNode(gen.Atom(fmt.Sprintf("distT11n2%s%s@localhost", suffix, uid)), options2)
	if err != nil {
		node1.Stop()
		t.Fatal(err)
	}

	// spawn receiver on node2
	receiverPID, err := node2.Spawn(factory_t11receiver, gen.ProcessOptions{})
	if err != nil {
		node1.Stop()
		node2.Stop()
		t.Fatal(err)
	}

	// establish connection
	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		node1.Stop()
		node2.Stop()
		t.Fatal(err)
	}

	return node1, node2, receiverPID
}
