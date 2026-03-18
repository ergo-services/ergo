package tm

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

func benchTM(nodeName string) (*targetManager, *mockCore) {
	core := newMockCore(nodeName)
	tm := Create(core, Options{}).(*targetManager)
	return tm, core
}

func makePID(node string, id uint64) gen.PID {
	return gen.PID{Node: gen.Atom(node), ID: id, Creation: 1}
}

func makeEvent(node string, name string) gen.Event {
	return gen.Event{Node: gen.Atom(node), Name: gen.Atom(name)}
}

func BenchmarkLinkPID_Local(b *testing.B) {
	tm, _ := benchTM("node1")
	target := makePID("node1", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		consumer := makePID("node1", uint64(i+1000))
		tm.LinkPID(consumer, target)
	}
}

func BenchmarkMonitorPID_Local(b *testing.B) {
	tm, _ := benchTM("node1")
	target := makePID("node1", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		consumer := makePID("node1", uint64(i+1000))
		tm.MonitorPID(consumer, target)
	}
}

func BenchmarkPublishEvent_NoSubscribers(b *testing.B) {
	tm, _ := benchTM("node1")
	producer := makePID("node1", 10)
	token, _ := tm.RegisterEvent(producer, "bench_event", gen.EventOptions{})
	event := makeEvent("node1", "bench_event")

	msg := gen.MessageEvent{Event: event}
	opts := gen.MessageOptions{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.PublishEvent(producer, token, opts, msg)
	}
}

func BenchmarkPublishEvent_10Subscribers(b *testing.B) {
	tm, _ := benchTM("node1")
	producer := makePID("node1", 10)
	token, _ := tm.RegisterEvent(producer, "bench_event", gen.EventOptions{})
	event := makeEvent("node1", "bench_event")

	for i := 0; i < 10; i++ {
		consumer := makePID("node1", uint64(i+100))
		tm.LinkEvent(consumer, event)
	}

	msg := gen.MessageEvent{Event: event}
	opts := gen.MessageOptions{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.PublishEvent(producer, token, opts, msg)
	}
}

func BenchmarkPublishEvent_100Subscribers(b *testing.B) {
	tm, _ := benchTM("node1")
	producer := makePID("node1", 10)
	token, _ := tm.RegisterEvent(producer, "bench_event", gen.EventOptions{})
	event := makeEvent("node1", "bench_event")

	for i := 0; i < 100; i++ {
		consumer := makePID("node1", uint64(i+100))
		tm.LinkEvent(consumer, event)
	}

	msg := gen.MessageEvent{Event: event}
	opts := gen.MessageOptions{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.PublishEvent(producer, token, opts, msg)
	}
}

func BenchmarkHasLink(b *testing.B) {
	tm, _ := benchTM("node1")
	consumer := makePID("node1", 1)
	target := makePID("node1", 100)
	tm.LinkPID(consumer, target)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.HasLink(consumer, target)
	}
}

func BenchmarkLinkPID_Parallel(b *testing.B) {
	tm, _ := benchTM("node1")

	b.RunParallel(func(pb *testing.PB) {
		id := uint64(0)
		for pb.Next() {
			id++
			consumer := makePID("node1", id+10000)
			target := makePID("node1", id)
			tm.LinkPID(consumer, target)
		}
	})
}

func BenchmarkPublishEvent_Parallel(b *testing.B) {
	tm, _ := benchTM("node1")
	producer := makePID("node1", 10)
	token, _ := tm.RegisterEvent(producer, "bench_event", gen.EventOptions{})
	event := makeEvent("node1", "bench_event")

	for i := 0; i < 10; i++ {
		consumer := makePID("node1", uint64(i+100))
		tm.LinkEvent(consumer, event)
	}

	msg := gen.MessageEvent{Event: event}
	opts := gen.MessageOptions{}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tm.PublishEvent(producer, token, opts, msg)
		}
	})
}

func BenchmarkContention_PublishWhileMonitor(b *testing.B) {
	tm, _ := benchTM("node1")

	type eventInfo struct {
		producer gen.PID
		token    gen.Ref
		event    gen.Event
	}
	var events []eventInfo

	for e := 0; e < 10; e++ {
		producer := makePID("node1", uint64(e+1))
		name := gen.Atom(fmt.Sprintf("event_%d", e))
		token, _ := tm.RegisterEvent(producer, name, gen.EventOptions{})
		event := gen.Event{Node: "node1", Name: name}

		for s := 0; s < 10; s++ {
			consumer := makePID("node1", uint64(e*100+s+1000))
			tm.LinkEvent(consumer, event)
		}

		events = append(events, eventInfo{producer: producer, token: token, event: event})
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		id := uint64(0)
		for pb.Next() {
			id++
			if id%2 == 0 {
				ei := events[id%uint64(len(events))]
				msg := gen.MessageEvent{Event: ei.event}
				tm.PublishEvent(ei.producer, ei.token, gen.MessageOptions{}, msg)
			} else {
				consumer := makePID("node1", id+50000)
				target := makePID("node1", id)
				tm.MonitorPID(consumer, target)
			}
		}
	})
}

func BenchmarkTerminatedTargetPID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tm, _ := benchTM("node1")
		target := makePID("node1", 100)

		for c := 0; c < 10; c++ {
			consumer := makePID("node1", uint64(c+1))
			tm.LinkPID(consumer, target)
		}

		b.StartTimer()
		tm.TerminatedTargetPID(target, fmt.Errorf("test"))
	}
}

func BenchmarkTerminatedProcess(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tm, _ := benchTM("node1")
		consumer := makePID("node1", 1)

		for t := 0; t < 100; t++ {
			target := makePID("node1", uint64(t+100))
			tm.LinkPID(consumer, target)
		}

		b.StartTimer()
		tm.TerminatedProcess(consumer, fmt.Errorf("test"))
	}
}

func BenchmarkTerminatedTargetNode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tm, _ := benchTM("node1")

		for c := 0; c < 100; c++ {
			consumer := makePID("node1", uint64(c+1))
			target := makePID("node2", uint64(c+1))
			s := tm.shardFor(target)
			key := relationKey{consumer: consumer, target: target}
			s.linkRelations[key] = struct{}{}
			entry := &targetEntry{consumers: make(map[gen.PID]struct{})}
			entry.consumers[consumer] = struct{}{}
			s.targetIndex[target] = entry
		}

		b.StartTimer()
		tm.TerminatedTargetNode("node2", fmt.Errorf("test"))
	}
}

func BenchmarkContention_TerminateNodeWhileMonitor(b *testing.B) {
	tm, _ := benchTM("node1")

	for c := 0; c < 100; c++ {
		consumer := makePID("node1", uint64(c+1))
		target := makePID("node2", uint64(c+1))
		s := tm.shardFor(target)
		key := relationKey{consumer: consumer, target: target}
		s.linkRelations[key] = struct{}{}
		entry := &targetEntry{consumers: make(map[gen.PID]struct{})}
		entry.consumers[consumer] = struct{}{}
		s.targetIndex[target] = entry
	}

	b.ResetTimer()

	var wg sync.WaitGroup
	wg.Add(1)

	done := make(chan struct{})
	go func() {
		defer wg.Done()
		id := uint64(0)
		for {
			select {
			case <-done:
				return
			default:
				id++
				consumer := makePID("node1", id+90000)
				target := makePID("node1", id+80000)
				tm.MonitorPID(consumer, target)
			}
		}
	}()

	for i := 0; i < b.N; i++ {
		tm.TerminatedTargetNode("node2", fmt.Errorf("test"))

		for c := 0; c < 100; c++ {
			consumer := makePID("node1", uint64(c+1))
			target := makePID("node2", uint64(c+1))
			s := tm.shardFor(target)
			s.mutex.Lock()
			key := relationKey{consumer: consumer, target: target}
			s.linkRelations[key] = struct{}{}
			entry := &targetEntry{consumers: make(map[gen.PID]struct{})}
			entry.consumers[consumer] = struct{}{}
			s.targetIndex[target] = entry
			s.mutex.Unlock()
		}
	}

	close(done)
	wg.Wait()
}

// BenchmarkMonitorLatencyUnderPublishLoad measures MonitorPID latency
// while N goroutines continuously publish events.
// This simulates the real bottleneck: sustained publish pressure starving MonitorPID.
func BenchmarkMonitorLatencyUnderPublishLoad(b *testing.B) {
	for _, publishers := range []int{1, 4, 8, 14} {
		b.Run(fmt.Sprintf("publishers=%d", publishers), func(b *testing.B) {
			tm, _ := benchTM("node1")

			// Register events, one per publisher
			type pubInfo struct {
				producer gen.PID
				token    gen.Ref
				event    gen.Event
				msg      gen.MessageEvent
			}
			pubs := make([]pubInfo, publishers)
			for i := 0; i < publishers; i++ {
				producer := makePID("node1", uint64(i+1))
				name := gen.Atom(fmt.Sprintf("event_%d", i))
				token, _ := tm.RegisterEvent(producer, name, gen.EventOptions{})
				event := gen.Event{Node: "node1", Name: name}

				// 20 subscribers per event
				for s := 0; s < 20; s++ {
					consumer := makePID("node1", uint64(i*100+s+1000))
					tm.LinkEvent(consumer, event)
				}

				pubs[i] = pubInfo{
					producer: producer,
					token:    token,
					event:    event,
					msg:      gen.MessageEvent{Event: event},
				}
			}

			// Start publishers - they publish non-stop
			stop := make(chan struct{})
			var publishCount atomic.Int64
			var publisherWg sync.WaitGroup

			for i := 0; i < publishers; i++ {
				publisherWg.Add(1)
				go func(p pubInfo) {
					defer publisherWg.Done()
					opts := gen.MessageOptions{}
					for {
						select {
						case <-stop:
							return
						default:
							tm.PublishEvent(p.producer, p.token, opts, p.msg)
							publishCount.Add(1)
						}
					}
				}(pubs[i])
			}

			// Let publishers warm up
			time.Sleep(10 * time.Millisecond)

			// Benchmark: measure MonitorPID latency under load
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				consumer := makePID("node1", uint64(i+500000))
				target := makePID("node1", uint64(i+600000))
				tm.MonitorPID(consumer, target)
			}
			b.StopTimer()

			close(stop)
			publisherWg.Wait()

			b.ReportMetric(float64(publishCount.Load())/float64(b.N), "publishes/monitor")
		})
	}
}
