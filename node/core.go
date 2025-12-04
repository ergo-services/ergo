package node

import (
	"sync/atomic"
	"time"

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

	return n.targets.PublishEvent(from, token, options, message)
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
			call.from = from
			call.important = options.ImportantDelivery

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

	resp := response{
		ref:       options.Ref,
		message:   message,
		from:      from,
		important: options.ImportantDelivery,
	}

	select {
	case p.response <- resp:
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
			call.from = from
			call.important = options.ImportantDelivery

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
	resp := response{
		ref:       options.Ref,
		err:       err,
		from:      from,
		important: options.ImportantDelivery,
	}

	select {
	case p.response <- resp:
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

	if target.Node == n.name {
		v, exist := n.processes.Load(target)
		if exist == false {
			return gen.ErrProcessUnknown
		}

		p := v.(*process)
		if p.State() == gen.ProcessStateTerminated {
			return gen.ErrProcessTerminated
		}
	}

	return n.targets.LinkPID(pid, target)
}

func (n *node) RouteUnlinkPID(pid gen.PID, target gen.PID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteUnlinkPID %s with %s ", pid, target)
	}

	return n.targets.UnlinkPID(pid, target)
}

func (n *node) RouteLinkProcessID(pid gen.PID, target gen.ProcessID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteLinkProcessID %s with %s", pid, target)
	}

	if target.Node == n.name {
		v, exist := n.names.Load(target.Name)
		if exist == false {
			return gen.ErrProcessUnknown
		}

		p := v.(*process)
		if p.State() == gen.ProcessStateTerminated {
			return gen.ErrProcessTerminated
		}
	}

	return n.targets.LinkProcessID(pid, target)
}

func (n *node) RouteUnlinkProcessID(pid gen.PID, target gen.ProcessID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	if lib.Trace() {
		n.log.Trace("RouteUnlinkProcessID %s with %s", pid, target)
	}

	return n.targets.UnlinkProcessID(pid, target)
}

func (n *node) RouteLinkAlias(pid gen.PID, target gen.Alias) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteLinkAlias %s with %s using %s", pid, target)
	}

	if target.Node == n.name {
		if _, exist := n.aliases.Load(target); exist == false {
			return gen.ErrAliasUnknown
		}
	}

	return n.targets.LinkAlias(pid, target)
}

func (n *node) RouteUnlinkAlias(pid gen.PID, target gen.Alias) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteUnlinkAlias %s with %s", pid, target)
	}

	return n.targets.UnlinkAlias(pid, target)
}

func (n *node) RouteLinkEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {

	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteLinkEvent %s with %s", pid, target)
	}

	return n.targets.LinkEvent(pid, target)
}

func (n *node) RouteUnlinkEvent(pid gen.PID, target gen.Event) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteUnlinkEvent %s with %s", pid, target)
	}

	return n.targets.UnlinkEvent(pid, target)
}

func (n *node) RouteMonitorPID(pid gen.PID, target gen.PID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteMonitorPID %s with %s", pid, target)
	}

	if target.Node == n.name {
		v, exist := n.processes.Load(target)
		if exist == false {
			return gen.ErrProcessUnknown
		}

		p := v.(*process)
		if p.State() == gen.ProcessStateTerminated {
			return gen.ErrProcessTerminated
		}
	}

	return n.targets.MonitorPID(pid, target)
}

func (n *node) RouteDemonitorPID(pid gen.PID, target gen.PID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteDemonitorPID %s with %s", pid, target)
	}

	return n.targets.DemonitorPID(pid, target)
}

func (n *node) RouteMonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteMonitorProcessID %s to %s", pid, target)
	}

	// Check if local target exists
	if target.Node == n.name {
		if v, exist := n.names.Load(target.Name); exist == false {
			return gen.ErrProcessUnknown
		} else {
			p := v.(*process)
			if p.State() == gen.ProcessStateTerminated {
				return gen.ErrProcessTerminated
			}
		}
	}

	return n.targets.MonitorProcessID(pid, target)
}

func (n *node) RouteDemonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteDemonitorProcessID %s to %s", pid, target)
	}

	return n.targets.DemonitorProcessID(pid, target)
}

func (n *node) RouteMonitorAlias(pid gen.PID, target gen.Alias) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteMonitorAlias %s to %s", pid, target)
	}

	if target.Node == n.name {
		if _, exist := n.aliases.Load(target); exist == false {
			return gen.ErrAliasUnknown
		}
	}

	return n.targets.MonitorAlias(pid, target)
}

func (n *node) RouteDemonitorAlias(pid gen.PID, target gen.Alias) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteDemonitorAlias %s to %s", pid, target)
	}

	return n.targets.DemonitorAlias(pid, target)
}

func (n *node) RouteMonitorEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {

	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteMonitorEvent %s to %s", pid, target)
	}

	return n.targets.MonitorEvent(pid, target)

}

func (n *node) RouteDemonitorEvent(pid gen.PID, target gen.Event) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteDemonitorEvent %s to %s", pid, target)
	}

	return n.targets.DemonitorEvent(pid, target)
}

func (n *node) RouteTerminatePID(target gen.PID, reason error) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteTerminatePID %s with reason %q", target, reason)
	}

	n.targets.TerminatedTargetPID(target, reason)

	return nil
}

func (n *node) RouteTerminateProcessID(target gen.ProcessID, reason error) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteTerminateProcessID %s with reason %q", target, reason)
	}

	n.targets.TerminatedTargetProcessID(target, reason)

	return nil
}

func (n *node) RouteTerminateEvent(target gen.Event, reason error) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteTerminateEvent %s with reason %q", target, reason)
	}

	n.targets.TerminatedTargetEvent(target, reason)

	return nil
}

func (n *node) RouteTerminateAlias(target gen.Alias, reason error) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteTerminateAlias %s with reason %q", target, reason)
	}

	n.targets.TerminatedTargetAlias(target, reason)

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

	// for local spawn via RouteSpawn, set deadline if not set
	if options.Ref == (gen.Ref{}) {
		timeout := options.InitTimeout
		if timeout == 0 {
			timeout = gen.DefaultRequestTimeout
		}
		deadline := time.Now().Unix() + int64(timeout)
		ref, err := n.MakeRefWithDeadline(deadline)
		if err != nil {
			return empty, err
		}
		options.Ref = ref
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

func (n *node) RouteApplicationInfo(name gen.Atom) (gen.ApplicationInfo, error) {
	if n.isRunning() == false {
		return gen.ApplicationInfo{}, gen.ErrNodeTerminated
	}

	if lib.Trace() {
		n.log.Trace("RouteApplicationInfo %s", name)
	}

	return n.ApplicationInfo(name)
}

func (n *node) RouteNodeDown(name gen.Atom, reason error) {
	if lib.Trace() {
		n.log.Trace("RouteNodeDown for %s ", name)
	}
	n.targets.TerminatedTargetNode(name, reason)
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

func (n *node) MakeRefWithDeadline(deadline int64) (gen.Ref, error) {
	if deadline < 1 {
		return gen.Ref{}, gen.ErrIncorrect
	}

	now := time.Now().Unix()
	if deadline > now {
		ref := n.MakeRef()
		ref.ID[2] = uint64(deadline)
		return ref, nil
	}

	return gen.Ref{}, gen.ErrIncorrect
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
