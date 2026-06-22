package distributed

import (
	"fmt"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// fragPong is a bare receiver; delivery of the reassembled message is observed on
// its node via the recorder (recordCore sees the fully reassembled value).
type fragPong struct{ act.Actor }

func factoryFragPong() gen.ProcessBehavior { return &fragPong{} }

// fragSender sends a value to a remote target with the per-process delivery option
// the test selects (ordered, unordered, compressed, important).
type fragSender struct{ act.Actor }

func factoryFragSender() gen.ProcessBehavior { return &fragSender{} }

type fragSendCmd struct {
	To    gen.PID
	Value any
	Mode  string
}

func (s *fragSender) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	c := request.(fragSendCmd)
	switch c.Mode {
	case "unordered":
		s.SetKeepNetworkOrder(false)
		defer s.SetKeepNetworkOrder(true)
		return errText(s.Send(c.To, c.Value)), nil
	case "compressed":
		s.SetCompression(true)
		defer s.SetCompression(false)
		return errText(s.Send(c.To, c.Value)), nil
	case "important":
		return errText(s.SendImportant(c.To, c.Value)), nil
	default: // "small", "ordered"
		return errText(s.Send(c.To, c.Value)), nil
	}
}

// TestDistFragmentation: a message larger than FragmentSize is split on the wire and
// reassembled intact on the peer. Covers ordered (single pool item) and unordered
// (round-robin across pool items) delivery, compression on top of fragmentation,
// important delivery of a fragmented message, and a sub-FragmentSize message that
// is delivered without fragmentation.
func TestDistFragmentation(t *testing.T) {
	s := stage.New(t)
	n1 := s.StartNode("n1", stage.NodeOptions{FragmentSize: 4096})
	n2 := s.StartNode("n2", stage.NodeOptions{FragmentSize: 4096})
	s.Connect(n1, n2)

	pong := n2.Spawn(factoryFragPong, gen.ProcessOptions{})
	sender := n1.Spawn(factoryFragSender, gen.ProcessOptions{})

	cases := []struct {
		mode string
		size int
	}{
		{"small", 5},
		{"ordered", 20000},
		{"unordered", 20000},
		{"compressed", 40000},
		{"important", 5000},
	}

	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			value := lib.RandomString(tc.size)
			mk := n2.Mark()
			res, err := n1.Call(sender, fragSendCmd{To: pong, Value: value, Mode: tc.mode})
			check.NoError(t, err)
			check.Equal(t, "", res)
			n2.ShouldDeliver().To(pong).Message(value).Since(mk).Once().Within(5 * time.Second).Must()
		})
	}
}

// fragLoadSender sends a batch of size-prefixed messages to a remote target. The
// first 10 chars of each payload encode its length so the receiver side can verify
// integrity after reassembly.
type fragLoadSender struct{ act.Actor }

func factoryFragLoadSender() gen.ProcessBehavior { return &fragLoadSender{} }

type fragBatch struct {
	To      gen.PID
	Size    int
	Count   int
	NoOrder bool
}

func (s *fragLoadSender) HandleMessage(from gen.PID, message any) error {
	b := message.(fragBatch)
	if b.NoOrder {
		s.SetKeepNetworkOrder(false)
	}
	for i := 0; i < b.Count; i++ {
		v := lib.RandomString(b.Size)
		v = fmt.Sprintf("%010d", len(v)) + v[10:]
		if err := s.Send(b.To, v); err != nil {
			return err
		}
	}
	return nil
}

// fragIntegrity matches a delivered payload whose length-prefix is intact, so a
// reassembly error (wrong length or corrupted prefix) fails the match.
func fragIntegrity(d check.Delivered) bool {
	s, ok := d.Message.(string)
	if ok == false || len(s) < 10 {
		return false
	}
	return s[:10] == fmt.Sprintf("%010d", len(s))
}

// TestDistFragmentationLoad: many senders concurrently stream fragmented messages
// to one receiver; every message must reassemble intact. Covers all-ordered,
// all-unordered (shared reassembly across pool items), and a mix of both.
func TestDistFragmentationLoad(t *testing.T) {
	const numSenders = 10
	const msgsPerSender = 100
	const total = numSenders * msgsPerSender

	run := func(t *testing.T, size func(idx int) int, noOrder func(idx int) bool) {
		s := stage.New(t)
		n1 := s.StartNode("n1", stage.NodeOptions{FragmentSize: 4096})
		n2 := s.StartNode("n2", stage.NodeOptions{FragmentSize: 4096})
		s.Connect(n1, n2)

		recv := n2.Spawn(factoryFragPong, gen.ProcessOptions{})
		mk := n2.Mark()
		for i := 0; i < numSenders; i++ {
			sender := n1.Spawn(factoryFragLoadSender, gen.ProcessOptions{})
			n1.Send(sender, fragBatch{To: recv, Size: size(i), Count: msgsPerSender, NoOrder: noOrder(i)})
		}
		n2.ShouldDeliver().To(recv).Where(fragIntegrity).Since(mk).Times(total).Within(30 * time.Second).Must()
	}

	t.Run("OrderedConcurrent", func(t *testing.T) {
		run(t, func(int) int { return 20000 }, func(int) bool { return false })
	})
	t.Run("UnorderedConcurrent", func(t *testing.T) {
		run(t, func(int) int { return 5000 }, func(int) bool { return true })
	})
	// odd senders: unordered larger messages; even senders: ordered smaller messages
	t.Run("MixedOrderConcurrent", func(t *testing.T) {
		run(t,
			func(idx int) int {
				if idx%2 == 1 {
					return 32000
				}
				return 12000
			},
			func(idx int) bool { return idx%2 == 1 },
		)
	})
}
