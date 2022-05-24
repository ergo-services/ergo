package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

const (
	defaultDispatcherBufferSize = 10000
	defaultAutoDemandCount      = 3
)

type StageProducerTest struct {
	gen.Stage
	value      chan interface{}
	dispatcher gen.StageDispatcherBehavior
}

type sendEvents struct {
	events etf.List
}
type cancelSubscription struct {
	subscription gen.StageSubscription
	reason       string
}

type demandRequest struct {
	subscription gen.StageSubscription
	count        uint
}
type newSubscription struct {
	producer etf.Term
	opts     gen.StageSubscribeOptions
}

//
// a simple Stage Producer
//

func (gs *StageProducerTest) InitStage(process *gen.StageProcess, args ...etf.Term) (gen.StageOptions, error) {
	opts := gen.StageOptions{
		Dispatcher: gs.dispatcher,
	}
	return opts, nil
}
func (gs *StageProducerTest) HandleDemand(process *gen.StageProcess, subscription gen.StageSubscription, count uint) (etf.List, gen.StageStatus) {
	gs.value <- etf.Tuple{subscription, count}
	return nil, gen.StageStatusOK
}

func (gs *StageProducerTest) HandleSubscribe(process *gen.StageProcess, subscription gen.StageSubscription, options gen.StageSubscribeOptions) gen.StageStatus {
	gs.value <- subscription
	return gen.StageStatusOK
}
func (gs *StageProducerTest) HandleCancel(process *gen.StageProcess, subscription gen.StageSubscription, reason string) gen.StageStatus {
	gs.value <- etf.Tuple{"cancel", subscription}
	return gen.StageStatusOK
}

// add this callback only for the 'not a producer' case
func (gs *StageProducerTest) HandleCanceled(process *gen.StageProcess, subscription gen.StageSubscription, reason string) gen.StageStatus {
	gs.value <- etf.Tuple{"canceled", subscription, reason}
	return gen.StageStatusOK
}

func (s *StageProducerTest) SendEvents(p gen.Process, events etf.List) error {
	message := sendEvents{
		events: events,
	}
	_, err := p.Direct(message)
	return err
}

func (s *StageProducerTest) Cancel(p gen.Process, subscription gen.StageSubscription, reason string) error {
	message := cancelSubscription{
		subscription: subscription,
		reason:       reason,
	}
	_, err := p.Direct(message)
	return err
}
func (gs *StageProducerTest) Subscribe(p gen.Process, producer etf.Term, opts gen.StageSubscribeOptions) (gen.StageSubscription, error) {
	message := newSubscription{
		producer: producer,
		opts:     opts,
	}
	s, err := p.Direct(message)
	if err != nil {
		return gen.StageSubscription{}, err
	}
	return s.(gen.StageSubscription), nil
}

func (s *StageProducerTest) HandleStageDirect(process *gen.StageProcess, ref etf.Ref, message interface{}) (interface{}, gen.DirectStatus) {
	switch m := message.(type) {
	case newSubscription:
		return process.Subscribe(m.producer, m.opts)
	case sendEvents:
		process.SendEvents(m.events)
		return nil, nil

	case cancelSubscription:
		err := process.Cancel(m.subscription, m.reason)
		return nil, err
	case makeCall:
		return process.Call(m.to, m.message)
	case makeCast:
		return nil, process.Cast(m.to, m.message)

	default:
		return nil, gen.ErrUnsupportedRequest
	}
}

//
// a simple Stage Consumer
//
type StageConsumerTest struct {
	gen.Stage
	value chan interface{}
}

func (gs *StageConsumerTest) InitStage(process *gen.StageProcess, args ...etf.Term) (gen.StageOptions, error) {
	return gen.StageOptions{}, nil
}
func (gs *StageConsumerTest) HandleEvents(process *gen.StageProcess, subscription gen.StageSubscription, events etf.List) gen.StageStatus {
	gs.value <- etf.Tuple{"events", subscription, events}
	return gen.StageStatusOK
}
func (gs *StageConsumerTest) HandleSubscribed(process *gen.StageProcess, subscription gen.StageSubscription, opts gen.StageSubscribeOptions) (bool, gen.StageStatus) {
	gs.value <- subscription
	return opts.ManualDemand, gen.StageStatusOK
}
func (gs *StageConsumerTest) HandleCanceled(process *gen.StageProcess, subscription gen.StageSubscription, reason string) gen.StageStatus {
	gs.value <- etf.Tuple{"canceled", subscription, reason}
	return gen.StageStatusOK
}
func (gs *StageConsumerTest) HandleStageCall(process *gen.StageProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	gs.value <- message
	return "ok", gen.ServerStatusOK
}
func (gs *StageConsumerTest) HandleStageCast(process *gen.StageProcess, message etf.Term) gen.ServerStatus {
	gs.value <- message
	return gen.ServerStatusOK
}
func (gs *StageConsumerTest) HandleStageInfo(process *gen.StageProcess, message etf.Term) gen.ServerStatus {
	gs.value <- message
	return gen.ServerStatusOK
}

func (s *StageConsumerTest) HandleStageDirect(p *gen.StageProcess, ref etf.Ref, message interface{}) (interface{}, gen.DirectStatus) {
	switch m := message.(type) {
	case newSubscription:
		return p.Subscribe(m.producer, m.opts)
	case demandRequest:
		err := p.Ask(m.subscription, m.count)
		return nil, err

	case cancelSubscription:
		err := p.Cancel(m.subscription, m.reason)
		return nil, err
	case makeCall:
		return p.Call(m.to, m.message)
	case makeCast:
		return nil, p.Cast(m.to, m.message)
	}
	return nil, gen.ErrUnsupportedRequest
}

func (gs *StageConsumerTest) Subscribe(p gen.Process, producer etf.Term, opts gen.StageSubscribeOptions) (gen.StageSubscription, error) {
	message := newSubscription{
		producer: producer,
		opts:     opts,
	}
	s, err := p.Direct(message)
	if err != nil {
		return gen.StageSubscription{}, err
	}
	return s.(gen.StageSubscription), nil
}

func (s *StageConsumerTest) Cancel(p gen.Process, subscription gen.StageSubscription, reason string) error {
	message := cancelSubscription{
		subscription: subscription,
		reason:       reason,
	}
	_, err := p.Direct(message)
	return err
}

func (s *StageConsumerTest) Ask(p gen.Process, subscription gen.StageSubscription, count uint) error {
	message := demandRequest{
		subscription: subscription,
		count:        count,
	}
	_, err := p.Direct(message)
	return err
}

func TestStageSimple(t *testing.T) {

	fmt.Printf("\n=== Test StageSimple\n")
	fmt.Printf("Starting node: nodeStageSimple01@localhost...")

	node, _ := ergo.StartNode("nodeStageSimple01@localhost", "cookies", node.Options{})

	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")

	producer := &StageProducerTest{
		value: make(chan interface{}, 2),
	}
	consumer := &StageConsumerTest{
		value: make(chan interface{}, 2),
	}
	//	producerProcess, _ :=
	fmt.Printf("... starting Producer and Consumer processes: ")
	producerProcess, errP := node.Spawn("stageProducer", gen.ProcessOptions{}, producer, nil)
	if errP != nil {
		t.Fatal(errP)
	}
	consumerProcess, errC := node.Spawn("stageConsumer", gen.ProcessOptions{}, consumer, nil)
	if errC != nil {
		t.Fatal(errC)
	}
	fmt.Println("OK")

	subOpts := gen.StageSubscribeOptions{
		MinDemand:    4,
		MaxDemand:    5,
		ManualDemand: true,
		// use Temporary to keep this process running
		Cancel: gen.StageCancelTemporary,
	}

	// case 1: subscribe
	fmt.Println("Subscribing/resubscribing/cancelation:")
	sub, _ := consumer.Subscribe(consumerProcess, "stageProducer", subOpts)
	fmt.Printf("... Producer handled subscription request from Consumer: ")
	waitForResultWithValue(t, producer.value, sub)
	fmt.Printf("... Consumer handled subscription confirmation from Producer: ")
	waitForResultWithValue(t, consumer.value, sub)
	// case 2: subscribe one more time (prev subscription should be canceled automatically)
	fmt.Printf("... Consumer subscribes to Producer without cancelation previous subscription: ")
	sub1, _ := consumer.Subscribe(consumerProcess, "stageProducer", subOpts)
	fmt.Println("OK")
	// previous subscription should be canceled
	fmt.Printf("... Producer canceled previous subscription and handled the new subscription request from Consumer: ")
	waitForResultWithValue(t, producer.value, sub1)
	fmt.Printf("... Consumer handled cancelation and subscription confirmation from Producer: ")
	waitForResultWithMultiValue(t, consumer.value, etf.List{sub1, etf.Tuple{"canceled", sub, "resubscribed"}})

	fmt.Printf("... Consumer invokes Cancel subscription explicitly:")
	if err := consumer.Cancel(consumerProcess, sub1, "normal"); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	fmt.Printf("... Producer handled cancelation request from Consumer: ")
	waitForResultWithValue(t, producer.value, etf.Tuple{"cancel", sub1})
	fmt.Printf("... Consumer handled cancelation confirmation from Producer: ")
	waitForResultWithValue(t, consumer.value, etf.Tuple{"canceled", sub1, "normal"})

	fmt.Println("make another subscription for the testing of explicit cancelation from the Producer side")
	sub2, _ := consumer.Subscribe(consumerProcess, "stageProducer", subOpts)
	fmt.Printf("... Producer handled subscription request from Consumer: ")
	waitForResultWithValue(t, producer.value, sub2)
	fmt.Printf("... Consumer handled subscription confirmation from Producer: ")
	waitForResultWithValue(t, consumer.value, sub2)
	if err := producer.Cancel(producerProcess, sub2, "normal"); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("... Producer handled cancelation request from itself: ")
	waitForResultWithValue(t, producer.value, etf.Tuple{"cancel", sub2})
	fmt.Printf("... Consumer handled cancelation confirmation from Producer: ")
	waitForResultWithValue(t, consumer.value, etf.Tuple{"canceled", sub2, "normal"})
	waitForTimeout(t, producer.value)
	waitForTimeout(t, consumer.value)

	// case 3:
	fmt.Printf("... trying to subscribe on Consumer (should fail with error 'not a producer'): ")
	// let's subscribe using Pid instead of registered name "stageConsumer"
	sub3, _ := producer.Subscribe(producerProcess, consumerProcess.Self(), subOpts)
	waitForResultWithValue(t, producer.value, etf.Tuple{"canceled", sub3, "not a producer"})

	// case 4: invoking Server callbacks
	fmt.Printf("... Invoking Server's callback handlers HandleStageCall: ")
	call := makeCall{
		to:      "stageConsumer",
		message: "test call",
	}
	if _, err := producerProcess.Direct(call); err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, consumer.value, "test call")
	fmt.Printf("... Invoking Server's callback handlers HandleStageCast: ")
	cast := makeCast{
		to:      "stageConsumer",
		message: "test cast",
	}
	producerProcess.Direct(cast)
	waitForResultWithValue(t, consumer.value, "test cast")
	fmt.Printf("... Invoking Server's callback handlers HandleStageInfo: ")
	producerProcess.Send("stageConsumer", "test info")
	waitForResultWithValue(t, consumer.value, "test info")

	// case 5:
	//    - subscribe, ask and send events
	//    - resubscribe with updated min/max, ask and send events
	//    keep this subscription for the next case
	subOpts.MinDemand = 2
	subOpts.MaxDemand = 4
	subOpts.ManualDemand = true
	fmt.Println("make yet another subscription for the testing subscribe/ask/send_events")
	sub4, _ := consumer.Subscribe(consumerProcess, producerProcess.Self(), subOpts)
	fmt.Printf("... Producer handled subscription request from Consumer: ")
	waitForResultWithValue(t, producer.value, sub4)
	fmt.Printf("... Consumer handled subscription confirmation from Producer: ")
	waitForResultWithValue(t, consumer.value, sub4)
	fmt.Printf("... Consumer sent 'Ask' request with count = 1 (min 2, max 4) : ")
	consumer.Ask(consumerProcess, sub4, 1)
	waitForResultWithValue(t, producer.value, etf.Tuple{sub4, uint(1)})
	waitForTimeout(t, consumer.value) // shouldn't receive anything here
	events := etf.List{
		"a", "b", "c", "d", "e",
	}
	fmt.Printf("... Producer sent 5 events. Consumer should receive 4: ")
	producer.SendEvents(producerProcess, events)
	expected := etf.Tuple{"events", sub4, events[0:4]}
	waitForResultWithValue(t, consumer.value, expected)
	events = etf.List{
		"f",
	}
	fmt.Printf("... Producer sent 1 event. Consumer shouldn't receive anything until make yet another 'Ask' request: ")
	producer.SendEvents(producerProcess, events)
	expected = etf.Tuple{"events", sub4, etf.List{"e", "f"}}
	waitForTimeout(t, consumer.value) // shouldn't receive anything here
	fmt.Println("OK")
	fmt.Printf("... Consumer sent 'Ask' request with count = 1 (min 2, max 4) : ")
	consumer.Ask(consumerProcess, sub4, 1)
	waitForResultWithValue(t, producer.value, etf.Tuple{sub4, uint(1)})
	fmt.Printf("... Consumer should receive 2 events: ")
	waitForResultWithValue(t, consumer.value, expected)

	// case 6:
	//    - ask, disable forwarding, send events, ask, enable forwarding
	//    keep this subscription for the next case

	fmt.Printf("... Disable forwarding demand on Producer: ")
	if err := producer.DisableForwardDemand(producerProcess); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	fmt.Printf("... Consumer sent 'Ask' request with count = 1 (min 2, max 4). Stage shouldn't call HandleDemand : ")
	consumer.Ask(consumerProcess, sub4, 1)
	waitForTimeout(t, producer.value) // shouldn't receive anything here
	fmt.Println("OK")

	events = etf.List{
		"a", "b", "c", "d", "e", "f", "g", "h", "1", "2",
	}
	producer.SendEvents(producerProcess, events)
	fmt.Printf("... Producer sent 10 events. Dispatcher should keep them all since demand was buffered: ")
	waitForTimeout(t, consumer.value) // shouldn't receive anything here
	fmt.Println("OK")
	fmt.Printf("... Consumer sent yet another 'Ask'. Stage shouldn't call HandleDemand as well: ")
	consumer.Ask(consumerProcess, sub4, 2)
	waitForTimeout(t, producer.value) // shouldn't receive anything here
	fmt.Println("OK")

	fmt.Printf("... Enable forwarding demand on Producer: ")
	if err := producer.EnableForwardDemand(producerProcess); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	fmt.Printf("... Producer should receive demands count =1 and count =2: ")
	expected1 := etf.Tuple{sub4, uint(1)}
	expected2 := etf.Tuple{sub4, uint(2)}
	waitForResultWithMultiValue(t, producer.value, etf.List{expected1, expected2})
	fmt.Printf("... Customer should receive 3 messages with 4, 4 and 2 events: ")
	expected1 = etf.Tuple{"events", sub4, etf.List{"a", "b", "c", "d"}}
	expected2 = etf.Tuple{"events", sub4, etf.List{"e", "f", "g", "h"}}
	expected3 := etf.Tuple{"events", sub4, etf.List{"1", "2"}}
	waitForResultWithMultiValue(t, consumer.value, etf.List{expected1, expected2, expected3})

	waitForTimeout(t, consumer.value) // shouldn't receive anything here
	waitForTimeout(t, producer.value) // shouldn't receive anything here

	// case 7:
	//    - enable auto demand, send events, try to ask (should get error), disable auto demands
	fmt.Printf("... Enable AutoDemand on the Producer (should fail): ")
	if err := producer.EnableAutoDemand(producerProcess, sub4); err == nil {
		t.Fatal("should fail here")
	}
	fmt.Println("OK")
	fmt.Printf("... Enable AutoDemand on the Consumer. Producer should receive demand with count %d: ", defaultAutoDemandCount)
	if err := consumer.EnableAutoDemand(consumerProcess, sub4); err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, producer.value, etf.Tuple{sub4, uint(3)})
	fmt.Printf("... Customer sent 'Ask' request (should fail): ")
	if err := consumer.Ask(consumerProcess, sub4, 2); err == nil {
		t.Fatal("should fail here")
	}
	fmt.Println("OK")
	events = etf.List{
		"a", "b", "c", "d", "e",
	}
	producer.SendEvents(producerProcess, events)
	fmt.Printf("... Producer sent 5 events. Consumer should receive 4. Demands counter = 2 now: ")
	expected = etf.Tuple{"events", sub4, etf.List{"a", "b", "c", "d"}}
	waitForResultWithValue(t, consumer.value, expected)
	fmt.Printf("... Consumer should send auto demand with count 3: ")
	waitForResultWithValue(t, producer.value, etf.Tuple{sub4, uint(3)})

	node.Stop()
}

func TestStageDistributed(t *testing.T) {
	fmt.Printf("\n=== Test StageDistributed\n")
	fmt.Printf("Starting node: nodeStageDistributed01@localhost...")
	node1, _ := ergo.StartNode("nodeStageDistributed01@localhost", "cookies", node.Options{})
	if node1 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
	fmt.Printf("Starting node: nodeStageDistributed02@localhost...")
	node2, _ := ergo.StartNode("nodeStageDistributed02@localhost", "cookies", node.Options{})
	if node2 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")

	producer := &StageProducerTest{
		value: make(chan interface{}, 2),
	}
	consumer := &StageConsumerTest{
		value: make(chan interface{}, 2),
	}
	fmt.Printf("... starting Producer and Consumer processes: ")
	producerProcess, errP := node1.Spawn("stageProducer", gen.ProcessOptions{}, producer, nil)
	if errP != nil {
		t.Fatal(errP)
	}
	consumerProcess, errC := node2.Spawn("stageConsumer", gen.ProcessOptions{}, consumer, nil)
	if errC != nil {
		t.Fatal(errC)
	}
	fmt.Println("OK")
	subOpts := gen.StageSubscribeOptions{
		MinDemand:    2,
		MaxDemand:    4,
		ManualDemand: true,
		// use Temporary to keep this process running
		Cancel: gen.StageCancelTemporary,
	}
	fmt.Println("Consumer@node2 subscribes on Producer@node1: ")
	sub, _ := consumer.Subscribe(consumerProcess, gen.ProcessID{Name: "stageProducer", Node: "nodeStageDistributed01@localhost"}, subOpts)
	fmt.Printf("... Producer@node1 handled subscription request from Consumer@node2: ")
	waitForResultWithValue(t, producer.value, sub)
	fmt.Printf("... Consumer@node2 handled subscription confirmation from Producer@node1: ")
	waitForResultWithValue(t, consumer.value, sub)

	fmt.Printf("... Consumer@node2 sent 'Ask' request with count = 1 (min 2, max 4) : ")
	consumer.Ask(consumerProcess, sub, 1)
	waitForResultWithValue(t, producer.value, etf.Tuple{sub, uint(1)})
	waitForTimeout(t, consumer.value) // shouldn't receive anything here
	events := etf.List{
		"a", "b", "c", "d", "e",
	}
	fmt.Printf("... Producer@node1 sent 5 events. Consumer@node2 should receive 4: ")
	producer.SendEvents(producerProcess, events)
	expected := etf.Tuple{"events", sub, events[0:4]}
	waitForResultWithValue(t, consumer.value, expected)

	producerProcess.Kill()
	fmt.Printf("... Producer process killed. Consumer should receive 'canceled' with reason 'kill': ")
	waitForResultWithValue(t, consumer.value, etf.Tuple{"canceled", sub, "kill"})
	if !consumerProcess.IsAlive() {
		t.Fatal("Consumer process should be alive here")
	}

	// case 2: StageCancelTransient

	fmt.Printf("... Starting Producer process (test StageCancelTransient) : ")
	producerProcess1, errP1 := node1.Spawn("stageProducer", gen.ProcessOptions{}, producer, nil)
	if errP1 != nil {
		t.Fatal(errP1)
	}
	fmt.Println("OK")

	subOpts.Cancel = gen.StageCancelTransient
	sub1, _ := consumer.Subscribe(consumerProcess, gen.ProcessID{Name: "stageProducer", Node: "nodeStageDistributed01@localhost"}, subOpts)
	fmt.Printf("... Producer@node1 handled subscription request from Consumer@node2: ")
	waitForResultWithValue(t, producer.value, sub1)
	fmt.Printf("... Consumer@node2 handled subscription confirmation from Producer@node1 (StageCancelTransient): ")
	waitForResultWithValue(t, consumer.value, sub1)

	producerProcess1.Kill()
	if err := producerProcess1.WaitWithTimeout(500 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("... Producer process killed. Consumer should receive 'canceled' with reason 'kill': ")
	waitForResultWithValue(t, consumer.value, etf.Tuple{"canceled", sub1, "kill"})
	fmt.Printf("... Consumer process should be terminated due to reason 'kill': ")
	if err := consumerProcess.WaitWithTimeout(500 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	// case 3: StageCancelPermanent

	fmt.Printf("... Starting Producer process (test StageCancelPermanent) : ")
	producerProcess2, errP2 := node1.Spawn("stageProducer", gen.ProcessOptions{}, producer, nil)
	if errP2 != nil {
		t.Fatal(errP2)
	}
	consumerProcess1, errC := node2.Spawn("stageConsumer", gen.ProcessOptions{}, consumer, nil)
	if errC != nil {
		t.Fatal(errC)
	}
	fmt.Println("OK")

	subOpts.Cancel = gen.StageCancelPermanent
	sub2, _ := consumer.Subscribe(consumerProcess1, gen.ProcessID{Name: "stageProducer", Node: "nodeStageDistributed01@localhost"}, subOpts)
	fmt.Printf("... Producer@node1 handled subscription request from Consumer@node2: ")
	waitForResultWithValue(t, producer.value, sub2)
	fmt.Printf("... Consumer@node2 handled subscription confirmation from Producer@node1 (StageCancelPermanent): ")
	waitForResultWithValue(t, consumer.value, sub2)

	producerProcess2.Exit("normal")
	if err := producerProcess2.WaitWithTimeout(500 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("... Producer process terminated normally. Consumer should receive 'canceled' with reason 'normal': ")
	waitForResultWithValue(t, consumer.value, etf.Tuple{"canceled", sub2, "normal"})
	fmt.Printf("... Consumer process should be terminated due to StageCancelPermanent mode: ")
	if err := consumerProcess1.WaitWithTimeout(500 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	// case 4: Cancel on producer's node termination
	fmt.Printf("... Starting Producer process (test cancel on node termination) : ")
	_, errP3 := node1.Spawn("stageProducer", gen.ProcessOptions{}, producer, nil)
	if errP3 != nil {
		t.Fatal(errP3)
	}
	consumerProcess2, errC := node2.Spawn("stageConsumer", gen.ProcessOptions{}, consumer, nil)
	if errC != nil {
		t.Fatal(errC)
	}
	fmt.Println("OK")
	subOpts.Cancel = gen.StageCancelTemporary
	sub3, err := consumer.Subscribe(consumerProcess2, gen.ProcessID{Name: "stageProducer", Node: "nodeStageDistributed01@localhost"}, subOpts)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("... Producer@node1 handled subscription request from Consumer@node2: ")
	waitForResultWithValue(t, producer.value, sub3)
	fmt.Printf("... Consumer@node2 handled subscription confirmation from Producer@node1 (StageCancelTemporary): ")
	waitForResultWithValue(t, consumer.value, sub3)

	node2.Disconnect(node1.Name())
	node1.Stop()
	fmt.Printf("... Stopping node1: ")
	if err := node1.WaitWithTimeout(1000 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	waitForResultWithValue(t, consumer.value, etf.Tuple{"canceled", sub3, "noconnection"})

	node2.Stop()
}

func TestStageDispatcherDemand(t *testing.T) {
	fmt.Printf("\n=== Test StageDispatcherDemand\n")
	fmt.Printf("Starting node: StageDispatcherDemand@localhost...")
	node, _ := ergo.StartNode("StageDispatcherDemand@localhost", "cookies", node.Options{})
	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")

	subOpts := gen.StageSubscribeOptions{
		MinDemand: 4,
		MaxDemand: 4,
	}

	producer := &StageProducerTest{
		value: make(chan interface{}, 2),
		// StageDispatcherDemand - this is default dispatcher, so we shouldn't create it explicitly
		// dispatcher: CreateStageDispatcherDemand(),
	}
	consumer := &StageConsumerTest{
		value: make(chan interface{}, 2),
	}
	fmt.Printf("... starting Producer (StageDispatcherDemand): ")
	producerProcess, errP := node.Spawn("stageProducer", gen.ProcessOptions{}, producer, nil)
	if errP != nil {
		t.Fatal(errP)
	}
	fmt.Println("OK")
	fmt.Printf("... starting Consumer1: ")
	consumer1Process, errC1 := node.Spawn("stageConsumer1", gen.ProcessOptions{}, consumer, nil)
	if errC1 != nil {
		t.Fatal(errC1)
	}
	fmt.Println("OK")
	sub1, _ := consumer.Subscribe(consumer1Process, "stageProducer", subOpts)
	fmt.Printf("... Producer handled subscription request from Consumer1: ")
	expected1 := etf.Tuple{sub1, uint(3)}
	expected2 := sub1
	waitForResultWithMultiValue(t, producer.value, etf.List{expected1, expected2})

	fmt.Printf("... Consumer1 handled subscription confirmation from Producer: ")
	waitForResultWithValue(t, consumer.value, sub1)

	fmt.Printf("... starting Consumer2: ")
	consumer2Process, errC2 := node.Spawn("stageConsumer2", gen.ProcessOptions{}, consumer, nil)
	if errC2 != nil {
		t.Fatal(errC2)
	}
	fmt.Println("OK")
	sub2, _ := consumer.Subscribe(consumer2Process, "stageProducer", subOpts)
	fmt.Printf("... Producer handled subscription request from Consumer2: ")
	expected1 = etf.Tuple{sub2, uint(3)}
	expected2 = sub2
	waitForResultWithMultiValue(t, producer.value, etf.List{expected1, expected2})
	fmt.Printf("... Consumer2 handled subscription confirmation from Producer: ")
	waitForResultWithValue(t, consumer.value, sub2)

	fmt.Printf("... starting Consumer3: ")
	consumer3Process, errC3 := node.Spawn("stageConsumer3", gen.ProcessOptions{}, consumer, nil)
	if errC3 != nil {
		t.Fatal(errC3)
	}
	fmt.Println("OK")
	sub3, _ := consumer.Subscribe(consumer3Process, "stageProducer", subOpts)
	fmt.Printf("... Producer handled subscription request from Consumer3: ")
	expected1 = etf.Tuple{sub3, uint(3)}
	expected2 = sub3
	waitForResultWithMultiValue(t, producer.value, etf.List{expected1, expected2})
	fmt.Printf("... Consumer3 handled subscription confirmation from Producer: ")
	waitForResultWithValue(t, consumer.value, sub3)

	fmt.Printf("... Producer sent 5 events. Consumer1 should receive 4: ")
	events := etf.List{
		"a", "b", "c", "d", "e",
	}
	producer.SendEvents(producerProcess, events)
	expected := etf.Tuple{"events", sub1, events[0:4]}
	waitForResultWithValue(t, consumer.value, expected)

	fmt.Printf("... Producer sent 5 events. Consumer2 should receive 4: ")
	events = etf.List{
		"f", "g", "h", "1", "2",
	}
	producer.SendEvents(producerProcess, events)
	expected = etf.Tuple{"events", sub2, etf.List{"e", "f", "g", "h"}}
	waitForResultWithValue(t, consumer.value, expected)

	fmt.Printf("... Producer sent 2 events. Consumer3 should receive 4: ")
	events = etf.List{
		"3", "4",
	}
	producer.SendEvents(producerProcess, events)
	expected = etf.Tuple{"events", sub3, etf.List{"1", "2", "3", "4"}}
	waitForResultWithValue(t, consumer.value, expected)

	node.Stop()
}

func TestStageDispatcherBroadcast(t *testing.T) {
	fmt.Printf("\n=== Test StageDispatcherBroadcast\n")
	fmt.Printf("Starting node: StageDispatcherBroadcast@localhost...")
	node, _ := ergo.StartNode("StageDispatcherBroadcast@localhost", "cookies", node.Options{})
	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")

	subOpts := gen.StageSubscribeOptions{
		MinDemand: 4,
		MaxDemand: 4,
	}

	producer := &StageProducerTest{
		value:      make(chan interface{}, 2),
		dispatcher: gen.CreateStageDispatcherBroadcast(),
	}
	consumer1 := &StageConsumerTest{
		value: make(chan interface{}, 2),
	}
	consumer2 := &StageConsumerTest{
		value: make(chan interface{}, 2),
	}
	consumer3 := &StageConsumerTest{
		value: make(chan interface{}, 2),
	}
	fmt.Printf("... starting Producer (StageDispatcherBroadcast): ")
	producerProcess, errP := node.Spawn("stageProducer", gen.ProcessOptions{}, producer, nil)
	if errP != nil {
		t.Fatal(errP)
	}
	fmt.Println("OK")
	fmt.Printf("... starting Consumer1: ")
	consumer1Process, errC1 := node.Spawn("stageConsumer1", gen.ProcessOptions{}, consumer1, nil)
	if errC1 != nil {
		t.Fatal(errC1)
	}
	fmt.Println("OK")
	sub1, _ := consumer1.Subscribe(consumer1Process, "stageProducer", subOpts)
	fmt.Printf("... Producer handled subscription request from Consumer1: ")
	expected1 := etf.Tuple{sub1, uint(3)}
	expected2 := sub1
	waitForResultWithMultiValue(t, producer.value, etf.List{expected1, expected2})

	fmt.Printf("... Consumer1 handled subscription confirmation from Producer: ")
	waitForResultWithValue(t, consumer1.value, sub1)

	fmt.Printf("... starting Consumer2: ")
	consumer2Process, errC2 := node.Spawn("stageConsumer2", gen.ProcessOptions{}, consumer2, nil)
	if errC2 != nil {
		t.Fatal(errC2)
	}
	fmt.Println("OK")
	sub2, _ := consumer2.Subscribe(consumer2Process, "stageProducer", subOpts)
	fmt.Printf("... Producer handled subscription request from Consumer2: ")
	expected1 = etf.Tuple{sub2, uint(3)}
	expected2 = sub2
	waitForResultWithMultiValue(t, producer.value, etf.List{expected1, expected2})
	fmt.Printf("... Consumer2 handled subscription confirmation from Producer: ")
	waitForResultWithValue(t, consumer2.value, sub2)

	fmt.Printf("... starting Consumer3: ")
	consumer3Process, errC3 := node.Spawn("stageConsumer3", gen.ProcessOptions{}, consumer3, nil)
	if errC3 != nil {
		t.Fatal(errC3)
	}
	fmt.Println("OK")
	sub3, _ := consumer3.Subscribe(consumer3Process, "stageProducer", subOpts)
	fmt.Printf("... Producer handled subscription request from Consumer3: ")
	expected1 = etf.Tuple{sub3, uint(3)}
	expected2 = sub3
	waitForResultWithMultiValue(t, producer.value, etf.List{expected1, expected2})
	fmt.Printf("... Consumer3 handled subscription confirmation from Producer: ")
	waitForResultWithValue(t, consumer3.value, sub3)

	fmt.Printf("... Producer sent 5 events:  ")
	events := etf.List{
		"a", "b", "c", "d", "e",
	}
	producer.SendEvents(producerProcess, events)
	fmt.Println("OK")
	exp1 := etf.Tuple{"events", sub1, events[0:4]}
	fmt.Printf("... Consumer1 should receive 4: ")
	waitForResultWithValue(t, consumer1.value, exp1)
	exp2 := etf.Tuple{"events", sub2, events[0:4]}
	fmt.Printf("... Consumer2 should receive 4: ")
	waitForResultWithValue(t, consumer2.value, exp2)
	exp3 := etf.Tuple{"events", sub3, events[0:4]}
	fmt.Printf("... Consumer2 should receive 4: ")
	waitForResultWithValue(t, consumer3.value, exp3)

	node.Stop()

}

func TestStageDispatcherPartition(t *testing.T) {
	fmt.Printf("\n=== Test StageDispatcherPartition\n")
	fmt.Printf("Starting node: StageDispatcherPartition@localhost...")
	node, _ := ergo.StartNode("StageDispatcherPartition@localhost", "cookies", node.Options{})
	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")

	subOpts1 := gen.StageSubscribeOptions{
		MinDemand: 3,
		MaxDemand: 4,
		Partition: 0,
	}
	subOpts2 := gen.StageSubscribeOptions{
		MinDemand: 3,
		MaxDemand: 4,
		Partition: 1,
	}
	subOpts3 := gen.StageSubscribeOptions{
		MinDemand: 3,
		MaxDemand: 4,
		Partition: 2,
	}

	hash := func(t etf.Term) int {
		i, ok := t.(int)
		if !ok {
			// filtering out
			return -1
		}

		if i > 1000 {
			return 2
		}
		if i > 100 {
			return 1
		}
		return 0
	}

	producer := &StageProducerTest{
		value:      make(chan interface{}, 2),
		dispatcher: gen.CreateStageDispatcherPartition(3, hash),
	}
	consumer1 := &StageConsumerTest{
		value: make(chan interface{}, 2),
	}
	consumer2 := &StageConsumerTest{
		value: make(chan interface{}, 2),
	}
	consumer3 := &StageConsumerTest{
		value: make(chan interface{}, 2),
	}

	fmt.Printf("... starting Producer (StageDispatcherPartition): ")
	producerProcess, errP := node.Spawn("stageProducer", gen.ProcessOptions{}, producer, nil)
	if errP != nil {
		t.Fatal(errP)
	}
	fmt.Println("OK")
	fmt.Printf("... starting Consumer1: ")
	consumer1Process, errC1 := node.Spawn("stageConsumer1", gen.ProcessOptions{}, consumer1, nil)
	if errC1 != nil {
		t.Fatal(errC1)
	}
	fmt.Println("OK")
	sub1, _ := consumer1.Subscribe(consumer1Process, "stageProducer", subOpts1)
	fmt.Printf("... Producer handled subscription request from Consumer1: ")
	expected1 := etf.Tuple{sub1, uint(3)}
	expected2 := sub1
	waitForResultWithMultiValue(t, producer.value, etf.List{expected1, expected2})

	fmt.Printf("... Consumer1 handled subscription confirmation from Producer: ")
	waitForResultWithValue(t, consumer1.value, sub1)

	fmt.Printf("... starting Consumer2: ")
	consumer2Process, errC2 := node.Spawn("stageConsumer2", gen.ProcessOptions{}, consumer2, nil)
	if errC2 != nil {
		t.Fatal(errC2)
	}
	fmt.Println("OK")
	sub2, _ := consumer2.Subscribe(consumer2Process, "stageProducer", subOpts2)
	fmt.Printf("... Producer handled subscription request from Consumer2: ")
	expected1 = etf.Tuple{sub2, uint(3)}
	expected2 = sub2
	waitForResultWithMultiValue(t, producer.value, etf.List{expected1, expected2})
	fmt.Printf("... Consumer2 handled subscription confirmation from Producer: ")
	waitForResultWithValue(t, consumer2.value, sub2)

	fmt.Printf("... starting Consumer3: ")
	consumer3Process, errC3 := node.Spawn("stageConsumer3", gen.ProcessOptions{}, consumer3, nil)
	if errC3 != nil {
		t.Fatal(errC3)
	}
	fmt.Println("OK")
	sub3, _ := consumer3.Subscribe(consumer3Process, "stageProducer", subOpts3)
	fmt.Printf("... Producer handled subscription request from Consumer3: ")
	expected1 = etf.Tuple{sub3, uint(3)}
	expected2 = sub3
	waitForResultWithMultiValue(t, producer.value, etf.List{expected1, expected2})
	fmt.Printf("... Consumer3 handled subscription confirmation from Producer: ")
	waitForResultWithValue(t, consumer3.value, sub3)

	fmt.Printf("... Producer sent 15 events. 3 of them should be discarded by 'hash' function:  ")
	events := etf.List{
		1, 2000, 200, "a", 90, "b", 80, 3000, 600, 9000, "c", 5, 1000, 100, 30,
	}
	producer.SendEvents(producerProcess, events)
	fmt.Println("OK")
	expEvents1 := etf.List{
		1, 90, 80, 5, // left in the queue: 100, 30,
	}
	exp1 := etf.Tuple{"events", sub1, expEvents1}
	fmt.Printf("... Consumer1 should receive 4: ")
	waitForResultWithValue(t, consumer1.value, exp1)
	expEvents2 := etf.List{
		200, 600, 1000,
	}
	exp2 := etf.Tuple{"events", sub2, expEvents2}
	fmt.Printf("... Consumer2 should receive 3: ")
	waitForResultWithValue(t, consumer2.value, exp2)
	expEvents3 := etf.List{
		2000, 3000, 9000,
	}
	exp3 := etf.Tuple{"events", sub3, expEvents3}
	fmt.Printf("... Consumer2 should receive 3: ")
	waitForResultWithValue(t, consumer3.value, exp3)

	node.Stop()

}
