package ergo

import (
	//"fmt"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
	"sync"
)

type GenStageSubscriptionMode string
type GenStageDemandMode string
type GenStageType int

type GenStageSubscriptionOptions struct {
	to        etf.Term
	minDemand int
	maxDemand int
}

// GenStageOptions defines the GenStage' configuration via Init callback.
// Some options are specific to the chosen stage mode while others are
// shared across all types.
type GenStageOptions struct {
	// mode defines the way of acting
	stageType GenStageType

	// ---- options for GenStageModeProducer

	// when set to GenStageDemandModeForward, the demand is always forwarded
	// to the HandleDemand callback. When this options is set to
	// GenStageDemandModeAccumulate, demands are accumulated until mode is
	// set back to GenStageDemandModeForward via Demand method
	demand GenStageDemandMode

	// ---- options for GenStageModeProducer and GenStageModeProducerConsumer

	// bufferSize the size of the buffer to store events without demand.
	// default is 10000
	bufferSize uint

	// bufferKeepFirst defines whether the first or last entries should be
	// kept on the buffer in case the buffer size is exceeded.
	bufferKeepFirst bool

	// ---- options for GenStageModeConsumer and GenStageModeProducerConsumer

	// subscribeTo a list of producers to subscribe to. Each element represents
	// a tuple with process of Producer (atom, tuple with registered name and node,
	// etf.Pid) and the subscription options.
	subscribeTo []etf.Term
}

const (
	GenStageSubscriptionModeAuto   GenStageSubscriptionMode = "automatic"
	GenStageSubscriptionModeManual GenStageSubscriptionMode = "manual"
	GenStageSubscriptionModeStop   GenStageSubscriptionMode = "stop"

	GenStageDemandModeForward    GenStageDemandMode = "forward"
	GenStageDemandModeAccumulate GenStageDemandMode = "accumulate"

	GenStageTypeProducer         GenStageType = 1
	GenStageTypeConsumer         GenStageType = 2
	GenStageTypeProducerConsumer GenStageType = 3
)

type GenStageBehavior interface {

	// Init(...) -> (GenStageOptions, state)
	Init(process *Process, args ...interface{}) (GenStageOptions, interface{})

	// HandleSubscribe()
	// HandleSubscribe -> (SubscriptionAuto | SubscriptionManual, State)

	// Invoked when a consumer is no longer subscribed to a producer.
	// HandleCancel -> ("noreply", subscription, state)
	//                 ("stop", reason, _)
	// where cancelReason is
	// 				etf.Tuple{"cancel", etf.Term(reason)}
	//              etf.Tuple{"down", etf.Term(reason)}
	// The cancelReason will be a {"cancel", _} tuple if the reason for cancellation
	// was a GenStage.cancel call. Any other value means the cancellation reason was
	// due to an EXIT.
	// Return values are the same as HandleCast.
	HandleCancel(cancelReason etf.Term, from etf.Term, state interface{}) (string, etf.Term, interface{})

	// invoked on GenStageTypeProducer
	// HandleDemand -> ("noreply", event, state)
	//                 ("stop", reason, _)
	HandleDemand(demand uint64, state interface{}) (string, etf.Term, interface{})

	HandleEvents(events interface{}, from etf.Term, state interface{}) (string, interface{})

	// This callback is invoked in both producers and consumers.
	// stageType will be GenStageTypeProducer if this callback is invoked on a consumer
	// and GenStageTypeConsumer if when this callback is invoked on producers a consumer subscribed to.
	// For consumers,  successful subscriptions must return one of:
	//   - (GenStageSubscriptionModeAuto, state)
	//      means the stage implementation will take care of automatically sending
	//      demand to producers. This is the default
	//   - (GenStageSubscriptionModeManual, state)
	//     means that demand must be sent to producers explicitly via Ask method.
	//     this kind of subscription must be canceled when HandleCancel is called
	// For producers, successful subscriptions must always return GenStageSubscriptionAuto.
	// Manual mode is not supported.
	HandleSubscribe(stageType GenStageType, options etf.List) (GenStageSubscriptionMode, interface{})

	// ----
	// GenStage has the same set of callback methods as a GenServer (except the Init one
	// since it a bit differs from the GenServer' Init method in returning value)
	// ----

	// HandleCast -> ("noreply", state) - noreply
	//		         ("stop", reason) - stop with reason
	HandleCast(message etf.Term, state interface{}) (string, interface{})
	// HandleCall -> ("reply", message, state) - reply
	//				 ("noreply", _, state) - noreply
	//		         ("stop", reason, _) - normal stop
	HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{})
	// HandleInfo -> ("noreply", state) - noreply
	//		         ("stop", reason) - normal stop
	HandleInfo(message etf.Term, state interface{}) (string, interface{})
	Terminate(reason string, state interface{})
}

type GenStage struct{}

func (gst *GenStage) loop(p *Process, object interface{}, args ...interface{}) string {
	var stageOptions GenStageOptions

	stageOptions, p.state = object.(GenStageBehavior).Init(p, args...)
	p.ready <- true

	stop := make(chan string, 2)

	p.currentFunction = "GenStage:loop"

	for {
		var message etf.Term
		var fromPid etf.Pid
		var lockState = &sync.Mutex{}

		select {
		case ex := <-p.gracefulExit:
			if p.trapExit {
				message = etf.Tuple{
					etf.Atom("EXIT"),
					ex.from,
					etf.Atom(ex.reason),
				}
			} else {
				object.(GenStageBehavior).Terminate(ex.reason, p.state)
				return ex.reason
			}
		case reason := <-stop:
			object.(GenStageBehavior).Terminate(reason, p.state)
			return reason
		case msg := <-p.mailBox:
			fromPid = msg.Element(1).(etf.Pid)
			message = msg.Element(2)
		case <-p.Context.Done():
			return "kill"
		case direct := <-p.direct:
			gst.handleDirect(direct)
			continue
		}

		lib.Log("[%s]. %v got message from %#v\n", p.Node.FullName, p.self, fromPid)

		p.reductions++

		switch m := message.(type) {
		case etf.Tuple:
			switch mtag := m.Element(1).(type) {
			case etf.Atom:
				switch mtag {
				case etf.Atom("$info"):

				case etf.Atom("$demand"):

				case etf.Atom("$subscribe"):

				case etf.Atom("$gen_consumer"):

				case etf.Atom("$gen_producer"):

				case etf.Atom("$gen_call"):
					go func() {
						fromTuple := m.Element(2).(etf.Tuple)
						lockState.Lock()

						cf := p.currentFunction
						p.currentFunction = "GenStage:HandleCall"
						code, reply, state := object.(GenStageBehavior).HandleCall(fromTuple, m.Element(3), p.state)
						p.currentFunction = cf

						if code == "stop" {
							stop <- reply.(string)
							// do not unlock, coz we have to keep this state unchanged for Terminate handler
							return
						}

						p.state = state
						lockState.Unlock()

						if reply != nil && code == "reply" {
							pid := fromTuple.Element(1).(etf.Pid)
							ref := fromTuple.Element(2)
							rep := etf.Term(etf.Tuple{ref, reply})
							p.Send(pid, rep)
						}
					}()

				case etf.Atom("$gen_cast"):
					go func() {
						lockState.Lock()

						cf := p.currentFunction
						p.currentFunction = "GenStage:HandleCast"
						code, state := object.(GenStageBehavior).HandleCast(m.Element(2), p.state)
						p.currentFunction = cf

						if code == "stop" {
							stop <- state.(string)
							return
						}
						p.state = state
						lockState.Unlock()
					}()

				default:
					go func() {
						lockState.Lock()

						cf := p.currentFunction
						p.currentFunction = "GenStage:HandleInfo"
						code, state := object.(GenStageBehavior).HandleInfo(message, p.state)
						p.currentFunction = cf

						if code == "stop" {
							stop <- state.(string)
							return
						}
						p.state = state
						lockState.Unlock()
					}()

				}

			case etf.Ref:
				lib.Log("got reply: %#v\n%#v", mtag, message)
				p.reply <- m

			default:
				lib.Log("mtag: %#v", mtag)
				go func() {
					lockState.Lock()

					cf := p.currentFunction
					p.currentFunction = "GenStage:HandleInfo"
					code, state := object.(GenStageBehavior).HandleInfo(message, p.state)
					p.currentFunction = cf

					if code == "stop" {
						stop <- state.(string)
					}
					p.state = state
					lockState.Unlock()
				}()
			}

		default:
			lib.Log("m: %#v", m)
			go func() {
				lockState.Lock()

				cf := p.currentFunction
				p.currentFunction = "GenStage:HandleInfo"
				code, state := object.(GenStageBehavior).HandleInfo(message, p.state)
				p.currentFunction = cf

				if code == "stop" {
					stop <- state.(string)
					return
				}
				p.state = state
				lockState.Unlock()
			}()
		}
	}
}

func (gst *GenStage) GetDemandMode() GenStageDemandMode {
	return GenStageDemandModeForward //FIXME
}

// SetDemandMode Sets the demand mode for a producer
// When DemandModeForward (by default), the demand is always forwarded to the handle_demand callback.
// When DemandModeAccumulate, demand is accumulated until its mode is set to DemandModeForward.
// This is useful as a synchronization mechanism, where the demand is accumulated until
// all consumers are subscribed.
func (gst *GenStage) SetDemandMode(mode GenStageDemandMode) {
}

func (gst *GenStage) handleDirect(m directMessage) {

	if m.reply != nil {
		m.err = ErrUnsupportedRequest
		m.reply <- m
	}
}
