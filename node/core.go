package node

import (
	"sync/atomic"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// gen.Core interface implementation

func (n *node) RouteSendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	var queue lib.QueueMPSC

	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteSendPID from %s to %s", from, to)
	}

	if to.Node != n.name {
		// remote
		connection, err := n.network.GetConnection(to.Node)
		if err != nil {
			return err
		}
		return connection.SendPID(from, to, options, message)
	}

	// local
	value, found := n.processes.Load(to)
	if found == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)

	if alive := p.isAlive(); alive == false {
		return gen.ErrProcessTerminated
	}

	switch options.Priority {
	case gen.MessagePriorityHigh:
		queue = p.mailbox.System
	case gen.MessagePriorityMax:
		queue = p.mailbox.Urgent
	default:
		queue = p.mailbox.Main
	}

	qm := gen.TakeMailboxMessage()
	qm.From = from
	qm.Type = gen.MailboxMessageTypeRegular
	qm.Target = to
	qm.Message = message

	if ok := queue.Push(qm); ok == false {
		if p.fallback.Enable == false {
			return gen.ErrProcessMailboxFull
		}

		if p.fallback.Name == p.name {
			return gen.ErrProcessMailboxFull
		}

		fbm := gen.MessageFallback{
			PID:     p.pid,
			Tag:     p.fallback.Tag,
			Message: message,
		}
		fbto := gen.ProcessID{Name: p.fallback.Name, Node: n.name}
		return n.RouteSendProcessID(from, fbto, options, fbm)
	}
	atomic.AddUint64(&p.messagesIn, 1)
	p.run()
	return nil
}

func (n *node) RouteSendProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	var queue lib.QueueMPSC

	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteSendProcessID from %s to %s", from, to)
	}

	if to.Node == "" {
		to.Node = n.name
	}

	if to.Node != n.name {
		// remote
		connection, err := n.network.GetConnection(to.Node)
		if err != nil {
			return err
		}
		return connection.SendProcessID(from, to, options, message)
	}

	value, found := n.names.Load(to.Name)
	if found == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)

	if alive := p.isAlive(); alive == false {
		return gen.ErrProcessTerminated
	}

	switch options.Priority {
	case gen.MessagePriorityHigh:
		queue = p.mailbox.System
	case gen.MessagePriorityMax:
		queue = p.mailbox.Urgent
	default:
		queue = p.mailbox.Main
	}

	qm := gen.TakeMailboxMessage()
	qm.From = from
	qm.Type = gen.MailboxMessageTypeRegular
	qm.Target = to.Name
	qm.Message = message

	if ok := queue.Push(qm); ok == false {
		if p.fallback.Enable == false {
			return gen.ErrProcessMailboxFull
		}

		if p.fallback.Name == p.name {
			return gen.ErrProcessMailboxFull
		}

		fbm := gen.MessageFallback{
			PID:     p.pid,
			Tag:     p.fallback.Tag,
			Message: message,
		}
		fbto := gen.ProcessID{Name: p.fallback.Name, Node: n.name}
		return n.RouteSendProcessID(from, fbto, options, fbm)
	}

	atomic.AddUint64(&p.messagesIn, 1)
	p.run()
	return nil
}

func (n *node) RouteSendAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {
	var queue lib.QueueMPSC

	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteSendAlias from %s to %s", from, to)
	}

	if to.Node != n.name {
		// remote
		connection, err := n.network.GetConnection(to.Node)
		if err != nil {
			return err
		}
		return connection.SendAlias(from, to, options, message)
	}

	value, found := n.aliases.Load(to)
	if found == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)

	if alive := p.isAlive(); alive == false {
		return gen.ErrProcessTerminated
	}

	qm := gen.TakeMailboxMessage()
	qm.From = from
	qm.Type = gen.MailboxMessageTypeRegular
	qm.Target = to
	qm.Message = message

	// check if this message should be delivered to the meta process
	if value, found := p.metas.Load(to); found {
		m := value.(*meta)
		if ok := m.main.Push(qm); ok == false {
			return gen.ErrMetaMailboxFull
		}
		atomic.AddUint64(&m.messagesIn, 1)
		m.handle()
		return nil
	}

	switch options.Priority {
	case gen.MessagePriorityHigh:
		queue = p.mailbox.System
	case gen.MessagePriorityMax:
		queue = p.mailbox.Urgent
	default:
		queue = p.mailbox.Main
	}

	if ok := queue.Push(qm); ok == false {
		if p.fallback.Enable == false {
			return gen.ErrProcessMailboxFull
		}

		if p.fallback.Name == p.name {
			return gen.ErrProcessMailboxFull
		}

		fbm := gen.MessageFallback{
			PID:     p.pid,
			Tag:     p.fallback.Tag,
			Message: message,
		}
		fbto := gen.ProcessID{Name: p.fallback.Name, Node: n.name}
		return n.RouteSendProcessID(from, fbto, options, fbm)
	}

	atomic.AddUint64(&p.messagesIn, 1)
	p.run()
	return nil
}

func (n *node) RouteSendEvent(from gen.PID, token gen.Ref, options gen.MessageOptions, message gen.MessageEvent) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteSendEvent from %s with token %s", from, token)
	}

	if from.Node == n.name {
		// local producer. check if sender is allowed to send this event
		value, found := n.events.Load(message.Event)
		if found == false {
			return gen.ErrEventUnknown
		}
		event := value.(*eventOwner)
		if event.token != token {
			return gen.ErrEventOwner
		}

		if event.last != nil {
			event.last.Push(message)
		}
	}

	consumers := n.targetManager.GetConsumersForTarget(message.Event)
	remote := make(map[gen.Atom]bool)
	// local delivery
	for _, pid := range consumers {
		if pid.Node == n.name {
			n.sendEventMessage(from, pid, options.Priority, message)
			continue
		}
		if from.Node != n.name {
			// event came here from the remote process. so there must be the local
			// subscribers only.  otherwise there is a bug
			panic("unable to route event from remote to the remote")
		}
		remote[pid.Node] = true
	}

	for k := range remote {
		// remote consumer means that there is a connection established
		// so use Connection() instead of GetConnection()
		connection, err := n.network.Connection(k)
		if err != nil {
			continue
		}
		if err := connection.SendEvent(from, options, message); err != nil {
			n.log.Error("unable to send event message to the remote consumer on %s: %s", k, err)
		}
	}
	return nil
}

func (n *node) RouteSendExit(from gen.PID, to gen.PID, reason error) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	if reason == nil {
		return gen.ErrIncorrect
	}

	if lib.Trace() {
		n.log.Trace("RouteSendExit from %s to %s with reason %q", from, to, reason)
	}

	if to.Node != n.name {
		// remote
		connection, err := n.network.GetConnection(to.Node)
		if err != nil {
			return err
		}
		return connection.SendExit(from, to, reason)
	}

	message := gen.MessageExitPID{
		PID:    from,
		Reason: reason,
	}
	return n.sendExitMessage(from, to, message)

}

func (n *node) RouteSendResponse(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteSendResponse from %s to %s with ref %q", from, to, options.Ref)
	}

	if to.Node != n.name {
		// remote
		connection, err := n.network.GetConnection(to.Node)
		if err != nil {
			return err
		}
		return connection.SendResponse(from, to, options, message)
	}

	// Check if this is a node-level call response
	if to == n.corePID {
		if value, found := n.calls.Load(options.Ref); found {
			call := value.(*nodeCall)
			call.response = message
			select {
			case call.done <- struct{}{}:
				return nil
			default:
				return gen.ErrResponseIgnored
			}
		}
		return gen.ErrResponseIgnored
	}

	value, loaded := n.processes.Load(to)
	if loaded == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)

	select {
	case p.response <- response{ref: options.Ref, message: message}:
		atomic.AddUint64(&p.messagesIn, 1)
		return nil
	default:
		// process doesn't wait for a response anymore
		return gen.ErrResponseIgnored
	}
}

func (n *node) RouteSendResponseError(from gen.PID, to gen.PID, options gen.MessageOptions, err error) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteSendResponseError from %s to %s with ref %q", from, to, options.Ref)
	}

	if to.Node != n.name {
		// remote
		connection, e := n.network.GetConnection(to.Node)
		if e != nil {
			return e
		}
		return connection.SendResponseError(from, to, options, err)
	}

	// Check if this is a node-level call response error
	if to == n.corePID {
		if value, found := n.calls.Load(options.Ref); found {
			call := value.(*nodeCall)
			call.err = err
			select {
			case call.done <- struct{}{}:
				return nil
			default:
				return gen.ErrResponseIgnored
			}
		}
		return gen.ErrResponseIgnored
	}

	value, loaded := n.processes.Load(to)
	if loaded == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)

	select {
	case p.response <- response{ref: options.Ref, err: err}:
		atomic.AddUint64(&p.messagesIn, 1)
		return nil
	default:
		// process doesn't wait for a response anymore
		return gen.ErrResponseIgnored
	}
}

func (n *node) RouteCallPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	var queue lib.QueueMPSC

	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	// not allowed to make a call request to itself
	if from == to {
		return gen.ErrNotAllowed
	}

	if lib.Trace() {
		n.log.Trace("RouteCallPID from %s to %s with ref %q", from, to, options.Ref)
	}

	if to.Node != n.name {
		// remote
		connection, err := n.network.GetConnection(to.Node)
		if err != nil {
			return err
		}
		return connection.CallPID(from, to, options, message)
	}

	// local
	value, found := n.processes.Load(to)
	if found == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)

	if alive := p.isAlive(); alive == false {
		return gen.ErrProcessTerminated
	}

	switch options.Priority {
	case gen.MessagePriorityHigh:
		queue = p.mailbox.System
	case gen.MessagePriorityMax:
		queue = p.mailbox.Urgent
	default:
		queue = p.mailbox.Main
	}

	qm := gen.TakeMailboxMessage()
	qm.Ref = options.Ref
	qm.From = from
	qm.Type = gen.MailboxMessageTypeRequest
	qm.Message = message

	if ok := queue.Push(qm); ok == false {
		return gen.ErrProcessMailboxFull
	}
	atomic.AddUint64(&p.messagesIn, 1)
	p.run()
	return nil
}

func (n *node) RouteCallProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	var queue lib.QueueMPSC

	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	if lib.Trace() {
		n.log.Trace("RouteCallProcessID from %s to %s with ref %q", from, to, options.Ref)
	}

	if to.Node != n.name {
		// remote
		connection, err := n.network.GetConnection(to.Node)
		if err != nil {
			return err
		}
		return connection.CallProcessID(from, to, options, message)
	}

	value, found := n.names.Load(to.Name)
	if found == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)
	if alive := p.isAlive(); alive == false {
		return gen.ErrProcessTerminated
	}

	switch options.Priority {
	case gen.MessagePriorityHigh:
		queue = p.mailbox.System
	case gen.MessagePriorityMax:
		queue = p.mailbox.Urgent
	default:
		queue = p.mailbox.Main
	}

	qm := gen.TakeMailboxMessage()
	qm.Ref = options.Ref
	qm.From = from
	qm.Type = gen.MailboxMessageTypeRequest
	qm.Target = to.Name
	qm.Message = message

	if ok := queue.Push(qm); ok == false {
		return gen.ErrProcessMailboxFull
	}
	atomic.AddUint64(&p.messagesIn, 1)
	p.run()
	return nil
}

func (n *node) RouteCallAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {
	var queue lib.QueueMPSC

	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteCallAlias from %s to %s with ref %q", from, to, options.Ref)
	}

	if to.Node != n.name {
		// remote
		connection, err := n.network.GetConnection(to.Node)
		if err != nil {
			return err
		}
		return connection.CallAlias(from, to, options, message)
	}

	value, found := n.aliases.Load(to)
	if found == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)
	if alive := p.isAlive(); alive == false {
		return gen.ErrProcessTerminated
	}

	qm := gen.TakeMailboxMessage()
	qm.Ref = options.Ref
	qm.From = from
	qm.Type = gen.MailboxMessageTypeRequest
	qm.Target = to
	qm.Message = message

	// check if this request should be delivered to the meta process
	if value, found := p.metas.Load(to); found {
		m := value.(*meta)
		if ok := m.main.Push(qm); ok == false {
			return gen.ErrMetaMailboxFull
		}
		atomic.AddUint64(&m.messagesIn, 1)
		m.handle()
		return nil
	}

	switch options.Priority {
	case gen.MessagePriorityHigh:
		queue = p.mailbox.System
	case gen.MessagePriorityMax:
		queue = p.mailbox.Urgent
	default:
		queue = p.mailbox.Main
	}
	if ok := queue.Push(qm); ok == false {
		return gen.ErrProcessMailboxFull
	}
	atomic.AddUint64(&p.messagesIn, 1)
	p.run()
	return nil
}

func (n *node) RouteLinkPID(pid gen.PID, target gen.PID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteLinkPID %s with %s", pid, target)
	}

	if n.name == target.Node {
		// local target
		if _, exist := n.processes.Load(target); exist == false {
			return gen.ErrProcessUnknown
		}
		return n.targetManager.AddLink(pid, target)
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.LinkPID(pid, target); err != nil {
		return err
	}

	return n.targetManager.AddLink(pid, target)
}

func (n *node) RouteUnlinkPID(pid gen.PID, target gen.PID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteUnlinkPID %s with %s ", pid, target)
	}

	if n.name == target.Node {
		// local target
		if _, exist := n.processes.Load(target); exist == false {
			return gen.ErrProcessUnknown
		}
		return n.targetManager.RemoveLink(pid, target)
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.UnlinkPID(pid, target); err != nil {
		return nil
	}

	return n.targetManager.RemoveLink(pid, target)
}

func (n *node) RouteLinkProcessID(pid gen.PID, target gen.ProcessID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteLinkProcessID %s with %s", pid, target)
	}

	if n.name == target.Node {
		// local target
		if _, exist := n.names.Load(target.Name); exist == false {
			return gen.ErrProcessUnknown
		}
		return n.targetManager.AddLink(pid, target)
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.LinkProcessID(pid, target); err != nil {
		return err
	}

	return n.targetManager.AddLink(pid, target)
}

func (n *node) RouteUnlinkProcessID(pid gen.PID, target gen.ProcessID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	if lib.Trace() {
		n.log.Trace("RouteUnlinkProcessID %s with %s", pid, target)
	}
	if n.name == target.Node {
		// local target
		if _, exist := n.names.Load(target.Name); exist == false {
			return gen.ErrProcessUnknown
		}
		return n.targetManager.RemoveLink(pid, target)
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.UnlinkProcessID(pid, target); err != nil {
		return err
	}
	return n.targetManager.RemoveLink(pid, target)
}

func (n *node) RouteLinkAlias(pid gen.PID, target gen.Alias) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteLinkAlias %s with %s using %s", pid, target)
	}

	if n.name == target.Node {
		// local target
		if _, exist := n.aliases.Load(target); exist == false {
			return gen.ErrAliasUnknown
		}
		return n.targetManager.AddLink(pid, target)
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.LinkAlias(pid, target); err != nil {
		return err
	}

	return n.targetManager.AddLink(pid, target)
}

func (n *node) RouteUnlinkAlias(pid gen.PID, target gen.Alias) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteUnlinkAlias %s with %s", pid, target)
	}

	if n.name == target.Node {
		// local target
		if _, exist := n.aliases.Load(target); exist == false {
			return gen.ErrAliasUnknown
		}
		return n.targetManager.RemoveLink(pid, target)
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.UnlinkAlias(pid, target); err != nil {
		return err
	}

	return n.targetManager.RemoveLink(pid, target)
}

func (n *node) RouteLinkEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {

	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteLinkEvent %s with %s", pid, target)
	}

	if n.name == target.Node {
		var lastEventMessages []gen.MessageEvent
		// local target
		value, exist := n.events.Load(target)
		if exist == false {
			return nil, gen.ErrEventUnknown
		}

		event := value.(*eventOwner)
		if err := n.targetManager.AddLink(pid, target); err != nil {
			return nil, err
		}

		if event.last != nil {
			// load last N events
			item := event.last.Item()
			for {
				if item == nil {
					break
				}
				v := item.Value().(gen.MessageEvent)
				lastEventMessages = append(lastEventMessages, v)
				item = item.Next()
			}
		}

		c := atomic.AddInt32(&event.consumers, 1)
		if event.notify == false || c > 1 {
			return lastEventMessages, nil
		}

		options := gen.MessageOptions{
			Priority: gen.MessagePriorityHigh,
		}
		message := gen.MessageEventStart{
			Name: target.Name,
		}
		n.RouteSendPID(n.corePID, event.producer, options, message)
		return lastEventMessages, nil
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return nil, err
	}

	lastEventMessages, err := connection.LinkEvent(pid, target)
	if err != nil {
		return nil, err
	}

	if err := n.targetManager.AddLink(pid, target); err != nil {
		return nil, err
	}

	return lastEventMessages, nil
}

func (n *node) RouteUnlinkEvent(pid gen.PID, target gen.Event) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteUnlinkEvent %s with %s", pid, target)
	}

	if n.name == target.Node {
		// local target
		value, exist := n.events.Load(target)
		if exist == false {
			return gen.ErrEventUnknown
		}
		event := value.(*eventOwner)
		if err := n.targetManager.RemoveLink(pid, target); err != nil {
			return err
		}

		c := atomic.AddInt32(&event.consumers, -1)
		if event.notify == false || c > 0 {
			return nil
		}

		// notify producer
		options := gen.MessageOptions{
			Priority: gen.MessagePriorityHigh,
		}
		message := gen.MessageEventStop{
			Name: target.Name,
		}
		n.RouteSendPID(n.corePID, event.producer, options, message)
		return nil
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.UnlinkEvent(pid, target); err != nil {
		return err
	}
	return n.targetManager.RemoveLink(pid, target)
}

func (n *node) RouteMonitorPID(pid gen.PID, target gen.PID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteMonitor %s to %s", pid, target)
	}

	if n.name == target.Node {
		// local target
		if v, exist := n.processes.Load(target); exist == false {
			return gen.ErrProcessUnknown
		} else {
			p := v.(*process)
			if p.State() == gen.ProcessStateTerminated {
				return gen.ErrProcessTerminated
			}
		}
		return n.targetManager.AddMonitor(pid, target)
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.MonitorPID(pid, target); err != nil {
		return err
	}
	return n.targetManager.AddMonitor(pid, target)
}

func (n *node) RouteDemonitorPID(pid gen.PID, target gen.PID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteDemonitor %s to %s", pid, target)
	}

	if n.name == target.Node {
		// local target
		if _, exist := n.processes.Load(target); exist == false {
			return gen.ErrProcessUnknown
		}
		return n.targetManager.RemoveMonitor(pid, target)
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.DemonitorPID(pid, target); err != nil {
		return err
	}
	return n.targetManager.RemoveMonitor(pid, target)
}

func (n *node) RouteMonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteMonitorProcessID %s to %s", pid, target)
	}

	if n.name == target.Node {
		// local target
		if v, exist := n.names.Load(target.Name); exist == false {
			return gen.ErrProcessUnknown
		} else {
			p := v.(*process)
			if p.State() == gen.ProcessStateTerminated {
				return gen.ErrProcessTerminated
			}
		}
		return n.targetManager.AddMonitor(pid, target)
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.MonitorProcessID(pid, target); err != nil {
		return err
	}
	return n.targetManager.AddMonitor(pid, target)
}

func (n *node) RouteDemonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteDemonitorProcessID %s to %s", pid, target)
	}

	if n.name == target.Node {
		// local target
		if _, exist := n.names.Load(target.Name); exist == false {
			return gen.ErrProcessUnknown
		}
		return n.targetManager.RemoveMonitor(pid, target)
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.DemonitorProcessID(pid, target); err != nil {
		return err
	}

	return n.targetManager.RemoveMonitor(pid, target)
}

func (n *node) RouteMonitorAlias(pid gen.PID, target gen.Alias) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteMonitorAlias %s to %s", pid, target)
	}

	if n.name == target.Node {
		// local target
		if _, exist := n.aliases.Load(target); exist == false {
			return gen.ErrAliasUnknown
		}
		return n.targetManager.AddMonitor(pid, target)
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.MonitorAlias(pid, target); err != nil {
		return err
	}

	return n.targetManager.AddMonitor(pid, target)
}

func (n *node) RouteDemonitorAlias(pid gen.PID, target gen.Alias) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteDemonitorAlias %s to %s", pid, target)
	}

	if n.name == target.Node {
		// local target
		if _, exist := n.aliases.Load(target); exist == false {
			return gen.ErrAliasUnknown
		}
		return n.targetManager.RemoveMonitor(pid, target)
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.DemonitorAlias(pid, target); err != nil {
		return err
	}

	return n.targetManager.RemoveMonitor(pid, target)
}

func (n *node) RouteMonitorEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {

	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteMonitorEvent %s to %s", pid, target)
	}

	if n.name == target.Node {
		var lastEventMessages []gen.MessageEvent
		// local target
		value, exist := n.events.Load(target)
		if exist == false {
			return nil, gen.ErrEventUnknown
		}
		event := value.(*eventOwner)
		if err := n.targetManager.AddMonitor(pid, target); err != nil {
			return nil, err
		}

		if event.last != nil {
			// load last N events
			item := event.last.Item()
			for {
				if item == nil {
					break
				}
				v := item.Value().(gen.MessageEvent)
				lastEventMessages = append(lastEventMessages, v)
				item = item.Next()
			}
		}

		c := atomic.AddInt32(&event.consumers, 1)
		if event.notify == false || c > 1 {
			return lastEventMessages, nil
		}

		options := gen.MessageOptions{
			Priority: gen.MessagePriorityHigh,
		}
		message := gen.MessageEventStart{
			Name: target.Name,
		}
		n.RouteSendPID(n.corePID, event.producer, options, message)
		return lastEventMessages, nil
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return nil, err
	}

	lastEventMessages, err := connection.MonitorEvent(pid, target)
	if err != nil {
		return nil, err
	}

	if err := n.targetManager.AddMonitor(pid, target); err != nil {
		return nil, err
	}
	return lastEventMessages, nil
}

func (n *node) RouteDemonitorEvent(pid gen.PID, target gen.Event) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteDemonitorEvent %s to %s", pid, target)
	}

	if n.name == target.Node {
		// local target
		value, exist := n.events.Load(target)
		if exist == false {
			return gen.ErrEventUnknown
		}

		if err := n.targetManager.RemoveMonitor(pid, target); err != nil {
			return err
		}

		// notify producer
		event := value.(*eventOwner)
		c := atomic.AddInt32(&event.consumers, -1)
		if event.notify == false || c > 0 {
			return nil
		}

		options := gen.MessageOptions{
			Priority: gen.MessagePriorityHigh,
		}
		message := gen.MessageEventStop{
			Name: target.Name,
		}
		n.RouteSendPID(n.corePID, event.producer, options, message)
		return nil
	}

	// remote target
	connection, err := n.network.GetConnection(target.Node)
	if err != nil {
		return err
	}

	if err := connection.DemonitorEvent(pid, target); err != nil {
		return err
	}

	return n.targetManager.RemoveMonitor(pid, target)
}

func (n *node) RouteTerminatePID(target gen.PID, reason error) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteTerminatePID %s with reason %q", target, reason)
	}

	remote := make(map[gen.Atom]bool)
	messageExit := gen.MessageExitPID{
		PID:    target,
		Reason: reason,
	}
	linkConsumers, monitorConsumers := n.targetManager.CleanupTarget(target)

	for _, pid := range linkConsumers {
		if pid.Node != n.name {
			remote[pid.Node] = true
		}
		n.sendExitMessage(target, pid, messageExit)
	}

	messageDown := gen.MessageDownPID{
		PID:    target,
		Reason: reason,
	}
	messageOptions := gen.MessageOptions{
		Priority: gen.MessagePriorityHigh,
	}
	for _, pid := range monitorConsumers {
		if pid.Node != n.name {
			remote[pid.Node] = true
		}
		n.RouteSendPID(target, pid, messageOptions, messageDown)
	}

	if target.Node != n.name && len(remote) > 0 {
		panic("bug")
	}

	for name := range remote {
		if connection, err := n.network.GetConnection(name); err == nil {
			connection.SendTerminatePID(target, reason)
		}
	}
	return nil
}

func (n *node) RouteTerminateProcessID(target gen.ProcessID, reason error) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteTerminateProcessID %s with reason %q", target, reason)
	}

	remote := make(map[gen.Atom]bool)
	messageExit := gen.MessageExitProcessID{
		ProcessID: target,
		Reason:    reason,
	}
	linkConsumers, monitorConsumers := n.targetManager.CleanupTarget(target)

	for _, pid := range linkConsumers {
		if pid.Node != n.name {
			remote[pid.Node] = true
		}
		n.sendExitMessage(n.corePID, pid, messageExit)
	}

	messageDown := gen.MessageDownProcessID{
		ProcessID: target,
		Reason:    reason,
	}
	messageOptions := gen.MessageOptions{
		Priority: gen.MessagePriorityHigh,
	}
	for _, pid := range monitorConsumers {
		if pid.Node != n.name {
			remote[pid.Node] = true
		}
		n.RouteSendPID(n.corePID, pid, messageOptions, messageDown)
	}

	if target.Node != n.name && len(remote) > 0 {
		panic("bug")
	}

	for name := range remote {
		if connection, err := n.network.GetConnection(name); err == nil {
			connection.SendTerminateProcessID(target, reason)
		}
	}
	return nil
}

func (n *node) RouteTerminateEvent(target gen.Event, reason error) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteTerminateEvent %s with reason %q", target, reason)
	}

	remote := make(map[gen.Atom]bool)
	messageExit := gen.MessageExitEvent{
		Event:  target,
		Reason: reason,
	}
	linkConsumers, monitorConsumers := n.targetManager.CleanupTarget(target)

	for _, pid := range linkConsumers {
		if pid.Node != n.name {
			remote[pid.Node] = true
		}
		n.sendExitMessage(n.corePID, pid, messageExit)
	}

	messageDown := gen.MessageDownEvent{
		Event:  target,
		Reason: reason,
	}
	messageOptions := gen.MessageOptions{
		Priority: gen.MessagePriorityHigh,
	}
	for _, pid := range monitorConsumers {
		if pid.Node != n.name {
			remote[pid.Node] = true
		}
		n.RouteSendPID(n.corePID, pid, messageOptions, messageDown)
	}

	if target.Node != n.name && len(remote) > 0 {
		panic("bug")
	}

	for name := range remote {
		if connection, err := n.network.GetConnection(name); err == nil {
			connection.SendTerminateEvent(target, reason)
		}
	}
	return nil
}

func (n *node) RouteTerminateAlias(target gen.Alias, reason error) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteTerminateAlias %s with reason %q", target, reason)
	}

	remote := make(map[gen.Atom]bool)
	messageExit := gen.MessageExitAlias{
		Alias:  target,
		Reason: reason,
	}
	linkConsumers, monitorConsumers := n.targetManager.CleanupTarget(target)

	for _, pid := range linkConsumers {
		if pid.Node != n.name {
			remote[pid.Node] = true
		}
		n.sendExitMessage(n.corePID, pid, messageExit)
	}

	messageDown := gen.MessageDownAlias{
		Alias:  target,
		Reason: reason,
	}
	messageOptions := gen.MessageOptions{
		Priority: gen.MessagePriorityHigh,
	}
	for _, pid := range monitorConsumers {
		if pid.Node != n.name {
			remote[pid.Node] = true
		}
		n.RouteSendPID(n.corePID, pid, messageOptions, messageDown)
	}

	if target.Node != n.name && len(remote) > 0 {
		panic("bug")
	}

	for name := range remote {
		if connection, err := n.network.GetConnection(name); err == nil {
			connection.SendTerminateAlias(target, reason)
		}
	}
	return nil
}

func (n *node) RouteSpawn(
	node gen.Atom,
	name gen.Atom,
	options gen.ProcessOptionsExtra,
	source gen.Atom,
) (gen.PID, error) {
	var empty gen.PID

	if n.isRunning() == false {
		return empty, gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteSpawn %s from %s to %s", name, options.ParentPID, node)
	}

	if node != n.name {
		// remote
		connection, err := n.network.GetConnection(node)
		if err != nil {
			return empty, err
		}
		return connection.RemoteSpawn(name, options)
	}

	factory, err := n.network.getEnabledSpawn(name, source)
	if err != nil {
		return empty, err
	}

	return n.spawn(factory, options)
}

func (n *node) RouteApplicationStart(
	name gen.Atom,
	mode gen.ApplicationMode,
	options gen.ApplicationOptionsExtra,
	source gen.Atom,
) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteApplicationStart %s with mode %s requested by %s", name, mode, source)
	}

	if err := n.network.isEnabledApplicationStart(name, source); err != nil {
		return err
	}

	v, exist := n.applications.Load(name)
	if exist == false {
		return gen.ErrApplicationUnknown
	}
	app := v.(*application)
	return app.start(mode, options)
}

func (n *node) RouteNodeDown(name gen.Atom, reason error) {
	// Get targets and consumers affected by node down, then cleanup
	linkTargetsWithConsumers, monitorTargetsWithConsumers := n.targetManager.CleanupNode(name)

	// Send exit messages for link targets that were cleaned up
	for target, linkConsumers := range linkTargetsWithConsumers {
		var message any
		switch t := target.(type) {
		case gen.PID:
			message = gen.MessageExitPID{
				PID:    t,
				Reason: gen.ErrNoConnection,
			}

		case gen.ProcessID:
			message = gen.MessageExitProcessID{
				ProcessID: t,
				Reason:    gen.ErrNoConnection,
			}

		case gen.Alias:
			message = gen.MessageExitAlias{
				Alias:  t,
				Reason: gen.ErrNoConnection,
			}

		case gen.Event:
			message = gen.MessageExitEvent{
				Event:  t,
				Reason: gen.ErrNoConnection,
			}

		case gen.Atom:
			message = gen.MessageExitNode{
				Name: name,
			}

		default:
			// bug
			continue
		}

		// Send exit messages to all consumers
		for _, pid := range linkConsumers {
			n.sendExitMessage(n.corePID, pid, message)
		}
	}

	// Send down messages for monitor targets that were cleaned up
	messageOptions := gen.MessageOptions{
		Priority: gen.MessagePriorityHigh,
	}
	for target, monitorConsumers := range monitorTargetsWithConsumers {
		var message any
		switch t := target.(type) {
		case gen.PID:
			message = gen.MessageDownPID{
				PID:    t,
				Reason: gen.ErrNoConnection,
			}

		case gen.ProcessID:
			message = gen.MessageDownProcessID{
				ProcessID: t,
				Reason:    gen.ErrNoConnection,
			}

		case gen.Alias:
			message = gen.MessageDownAlias{
				Alias:  t,
				Reason: gen.ErrNoConnection,
			}

		case gen.Event:
			message = gen.MessageDownEvent{
				Event:  t,
				Reason: gen.ErrNoConnection,
			}

		case gen.Atom:
			message = gen.MessageDownNode{
				Name: name,
			}

		default:
			// bug
			continue
		}

		// Send down messages to all consumers
		for _, pid := range monitorConsumers {
			n.RouteSendPID(n.corePID, pid, messageOptions, message)
		}
	}
}

func (n *node) MakeRef() gen.Ref {
	var ref gen.Ref
	ref.Node = n.name
	ref.Creation = n.creation
	id := atomic.AddUint64(&n.uniqID, 1)
	ref.ID[0] = id & ((2 << 17) - 1)
	ref.ID[1] = id >> 46
	return ref
}

func (n *node) PID() gen.PID {
	return n.corePID
}

func (n *node) LogLevel() gen.LogLevel {
	return n.log.Level()
}

func (n *node) Creation() int64 {
	return n.creation
}

func (n *node) sendExitMessage(from gen.PID, to gen.PID, message any) error {
	value, loaded := n.processes.Load(to)
	if loaded == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)

	if lib.Trace() {
		n.log.Trace("...sendExitMessage from %s to %s ", from, to)
	}

	// graceful shutdown via messaging
	qm := gen.TakeMailboxMessage()
	qm.From = from
	qm.Type = gen.MailboxMessageTypeExit
	qm.Message = message

	if ok := p.mailbox.Urgent.Push(qm); ok == false {
		return gen.ErrProcessMailboxFull
	}

	atomic.AddUint64(&p.messagesIn, 1)
	p.run()
	return nil
}

func (n *node) sendEventMessage(
	from gen.PID,
	to gen.PID,
	priority gen.MessagePriority,
	message gen.MessageEvent,
) error {
	var queue lib.QueueMPSC

	value, loaded := n.processes.Load(to)
	if loaded == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)

	switch priority {
	case gen.MessagePriorityHigh:
		queue = p.mailbox.System
	case gen.MessagePriorityMax:
		queue = p.mailbox.Urgent
	default:
		queue = p.mailbox.Main
	}

	if lib.Trace() {
		n.log.Trace("...sendEventMessage from %s to %s ", from, to)
	}

	qm := gen.TakeMailboxMessage()
	qm.From = from
	qm.Type = gen.MailboxMessageTypeEvent
	qm.Message = message

	if ok := queue.Push(qm); ok == false {
		return gen.ErrProcessMailboxFull
	}

	atomic.AddUint64(&p.messagesIn, 1)
	p.run()
	return nil
}
