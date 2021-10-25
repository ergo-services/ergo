package node

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

const (
	startPID = 1000
)

type registrar struct {
	monitor
	ctx context.Context

	nextPID  uint64
	uniqID   uint64
	nodename string
	creation uint32

	node nodeInternal

	names            map[string]etf.Pid
	mutexNames       sync.Mutex
	aliases          map[etf.Alias]*process
	mutexAliases     sync.Mutex
	processes        map[uint64]*process
	mutexProcesses   sync.Mutex
	connections      map[string]*Connection
	mutexConnections sync.Mutex

	behaviors      map[string]map[string]gen.RegisteredBehavior
	mutexBehaviors sync.Mutex
}

type registrarInternal interface {
	gen.Registrar
	Router
	monitorInternal

	spawn(name string, opts processOptions, behavior gen.ProcessBehavior, args ...etf.Term) (gen.Process, error)
	registerName(name string, pid etf.Pid) error
	unregisterName(name string) error
	registerPeer(peer *peer) error
	unregisterPeer(name string)
	newAlias(p *process) (etf.Alias, error)
	deleteAlias(owner *process, alias etf.Alias) error
	getProcessByPid(etf.Pid) *process

	route(from etf.Pid, to etf.Term, message etf.Term) error
	routeRaw(nodename etf.Atom, messages ...etf.Term) error
}

func newRegistrar(ctx context.Context, nodename string, creation uint32, node nodeInternal) registrarInternal {
	r := &registrar{
		ctx:     ctx,
		nextPID: startPID,
		uniqID:  uint64(time.Now().UnixNano()),
		// keep node to get the process to access to the node's methods
		node:        node.(nodeInternal),
		nodename:    nodename,
		creation:    creation,
		names:       make(map[string]etf.Pid),
		aliases:     make(map[etf.Alias]*process),
		processes:   make(map[uint64]*process),
		connections: make(map[string]*Connection),
		behaviors:   make(map[string]map[string]gen.RegisteredBehavior),
	}
	r.monitor = newMonitor(r)
	return r
}

// NodeName
func (r *registrar) NodeName() string {
	return r.nodename
}

// NodeStop
func (r *registrar) NodeStop() {
	r.node.Stop()
}

func (r *registrar) newPID() etf.Pid {
	// http://erlang.org/doc/apps/erts/erl_ext_dist.html#pid_ext
	// https://stackoverflow.com/questions/243363/can-someone-explain-the-structure-of-a-pid-in-erlang
	i := atomic.AddUint64(&r.nextPID, 1)
	return etf.Pid{
		Node:     etf.Atom(r.nodename),
		ID:       i,
		Creation: r.creation,
	}

}

// MakeRef returns atomic reference etf.Ref within this node
func (r *registrar) MakeRef() (ref etf.Ref) {
	ref.Node = etf.Atom(r.nodename)
	ref.Creation = r.creation
	nt := atomic.AddUint64(&r.uniqID, 1)
	ref.ID[0] = uint32(uint64(nt) & ((2 << 17) - 1))
	ref.ID[1] = uint32(uint64(nt) >> 46)

	return
}

// IsAlias
func (r *registrar) IsAlias(alias etf.Alias) bool {
	r.mutexAliases.Lock()
	_, ok := r.aliases[alias]
	r.mutexAliases.Unlock()
	return ok
}

func (r *registrar) newAlias(p *process) (etf.Alias, error) {
	var alias etf.Alias

	// chech if its alive
	r.mutexProcesses.Lock()
	_, exist := r.processes[p.self.ID]
	r.mutexProcesses.Unlock()
	if !exist {
		return alias, ErrProcessUnknown
	}

	alias = etf.Alias(r.MakeRef())
	lib.Log("[%s] REGISTRAR create process alias for %v: %s", r.nodename, p.self, alias)

	r.mutexAliases.Lock()
	r.aliases[alias] = p
	r.mutexAliases.Unlock()

	p.Lock()
	p.aliases = append(p.aliases, alias)
	p.Unlock()
	return alias, nil
}

func (r *registrar) deleteAlias(owner *process, alias etf.Alias) error {
	lib.Log("[%s] REGISTRAR delete process alias %v for %v", r.nodename, alias, owner.self)

	r.mutexAliases.Lock()
	p, alias_exist := r.aliases[alias]
	r.mutexAliases.Unlock()

	if !alias_exist {
		return ErrAliasUnknown
	}

	r.mutexProcesses.Lock()
	_, process_exist := r.processes[owner.self.ID]
	r.mutexProcesses.Unlock()

	if !process_exist {
		return ErrProcessUnknown
	}
	if p.self != owner.self {
		return ErrAliasOwner
	}

	p.Lock()
	for i := range p.aliases {
		if alias != p.aliases[i] {
			continue
		}
		delete(r.aliases, alias)
		p.aliases[i] = p.aliases[0]
		p.aliases = p.aliases[1:]
		p.Unlock()
		return nil
	}

	p.Unlock()
	fmt.Println("Bug: Process lost its alias. Please, report this issue")
	r.mutexAliases.Lock()
	delete(r.aliases, alias)
	r.mutexAliases.Unlock()

	return ErrAliasUnknown
}

func (r *registrar) newProcess(name string, behavior gen.ProcessBehavior, opts processOptions) (*process, error) {

	var parentContext context.Context

	mailboxSize := DefaultProcessMailboxSize
	if opts.MailboxSize > 0 {
		mailboxSize = int(opts.MailboxSize)
	}

	parentContext = r.ctx

	processContext, kill := context.WithCancel(parentContext)
	if opts.Context != nil {
		processContext, _ = context.WithCancel(opts.Context)
	}

	pid := r.newPID()

	// set global variable 'node'
	if opts.Env == nil {
		opts.Env = make(map[string]interface{})
	}
	opts.Env["ergo:Node"] = r.node.(Node)

	process := &process{
		registrarInternal: r,

		self:     pid,
		name:     name,
		behavior: behavior,
		env:      opts.Env,

		parent:      opts.parent,
		groupLeader: opts.GroupLeader,

		mailBox:      make(chan gen.ProcessMailboxMessage, mailboxSize),
		gracefulExit: make(chan gen.ProcessGracefulExitRequest, mailboxSize),
		direct:       make(chan gen.ProcessDirectMessage),

		context: processContext,
		kill:    kill,

		reply: make(map[etf.Ref]chan etf.Term),
	}

	process.exit = func(from etf.Pid, reason string) error {
		lib.Log("[%s] EXIT from %s to %s with reason: %s", r.nodename, from, pid, reason)
		if processContext.Err() != nil {
			// process is already died
			return ErrProcessUnknown
		}

		ex := gen.ProcessGracefulExitRequest{
			From:   from,
			Reason: reason,
		}

		// use select just in case if this process isn't been started yet
		// or ProcessLoop is already exited (has been set to nil)
		// otherwise it cause infinity lock
		select {
		case process.gracefulExit <- ex:
		default:
			return ErrProcessBusy
		}

		// let the process decide whether to stop itself, otherwise its going to be killed
		if !process.trapExit {
			process.kill()
		}
		return nil
	}

	if name != "" {
		lib.Log("[%s] REGISTRAR registering name (%s): %s", r.nodename, pid, name)
		r.mutexNames.Lock()
		if _, exist := r.names[name]; exist {
			r.mutexNames.Unlock()
			return nil, ErrTaken
		}
		r.names[name] = process.self
		r.mutexNames.Unlock()
	}

	lib.Log("[%s] REGISTRAR registering process: %s", r.nodename, pid)
	r.mutexProcesses.Lock()
	r.processes[process.self.ID] = process
	r.mutexProcesses.Unlock()

	return process, nil
}

func (r *registrar) deleteProcess(pid etf.Pid) {
	r.mutexProcesses.Lock()
	p, exist := r.processes[pid.ID]
	if !exist {
		r.mutexProcesses.Unlock()
		return
	}
	lib.Log("[%s] REGISTRAR unregistering process: %s", r.nodename, p.self)
	delete(r.processes, pid.ID)
	r.mutexProcesses.Unlock()

	r.mutexNames.Lock()
	if (p.name) != "" {
		lib.Log("[%s] REGISTRAR unregistering name (%s): %s", r.nodename, p.self, p.name)
		delete(r.names, p.name)
	}

	// delete names registered with this pid
	for name, pid := range r.names {
		if p.self == pid {
			delete(r.names, name)
		}
	}
	r.mutexNames.Unlock()

	r.mutexAliases.Lock()
	for alias := range r.aliases {
		delete(r.aliases, alias)
	}
	r.mutexAliases.Unlock()

	return
}

func (r *registrar) spawn(name string, opts processOptions, behavior gen.ProcessBehavior, args ...etf.Term) (gen.Process, error) {

	process, err := r.newProcess(name, behavior, opts)
	if err != nil {
		return nil, err
	}
	lib.Log("[%s] REGISTRAR spawn a new process %s (registered name: %q)", r.nodename, process.self, name)

	initProcess := func() (ps gen.ProcessState, err error) {
		if lib.CatchPanic() {
			defer func() {
				if rcv := recover(); rcv != nil {
					pc, fn, line, _ := runtime.Caller(2)
					fmt.Printf("Warning: initialization process failed %s[%q] %#v at %s[%s:%d]\n",
						process.self, name, rcv, runtime.FuncForPC(pc).Name(), fn, line)
					r.deleteProcess(process.self)
					err = fmt.Errorf("panic")
				}
			}()
		}

		ps, err = behavior.ProcessInit(process, args...)
		return
	}

	processState, err := initProcess()
	if err != nil {
		return nil, err
	}

	started := make(chan bool)
	defer close(started)

	cleanProcess := func(reason string) {
		// set gracefulExit to nil before we start termination handling
		process.gracefulExit = nil
		r.deleteProcess(process.self)
		// invoke cancel context to prevent memory leaks
		// and propagate context canelation
		process.Kill()
		// notify all the linked process and monitors
		r.handleTerminated(process.self, name, reason)
		// make the rest empty
		process.Lock()
		process.aliases = []etf.Alias{}

		// Do not clean self and name. Sometimes its good to know what pid
		// (and what name) was used by the dead process. (gen.Applications is using it)
		// process.name = ""
		// process.self = etf.Pid{}

		process.behavior = nil
		process.parent = nil
		process.groupLeader = nil
		process.exit = nil
		process.kill = nil
		process.mailBox = nil
		process.direct = nil
		process.env = nil
		process.reply = nil
		process.Unlock()
	}

	go func(ps gen.ProcessState) {
		if lib.CatchPanic() {
			defer func() {
				if rcv := recover(); rcv != nil {
					pc, fn, line, _ := runtime.Caller(2)
					fmt.Printf("Warning: process terminated %s[%q] %#v at %s[%s:%d]\n",
						process.self, name, rcv, runtime.FuncForPC(pc).Name(), fn, line)
					cleanProcess("panic")
				}
			}()
		}

		// start process loop
		reason := behavior.ProcessLoop(ps, started)
		// process stopped
		cleanProcess(reason)

	}(processState)

	// wait for the starting process loop
	<-started
	return process, nil
}

func (r *registrar) registerName(name string, pid etf.Pid) error {
	lib.Log("[%s] REGISTRAR registering name %s", r.nodename, name)
	r.mutexNames.Lock()
	defer r.mutexNames.Unlock()
	if _, ok := r.names[name]; ok {
		// already registered
		return ErrTaken
	}
	r.names[name] = pid
	return nil
}

func (r *registrar) unregisterName(name string) error {
	lib.Log("[%s] REGISTRAR unregistering name %s", r.nodename, name)
	r.mutexNames.Lock()
	defer r.mutexNames.Unlock()
	if _, ok := r.names[name]; ok {
		delete(r.names, name)
		return nil
	}
	return ErrNameUnknown
}

func (r *registrar) registerConnection(c *Connection) error {
	lib.Log("[%s] REGISTRAR registering peer %#v", r.nodename, c.Name)
	r.mutexConnections.Lock()
	defer r.mutexConnection.Unlock()
	// FIXME validate c.Name. must be abc@def format

	if _, ok := r.connections[c.Name]; ok {
		// already registered
		return ErrTaken
	}
	r.connections[c.Name] = c
	return nil
}

func (r *registrar) unregisterConnection(name string) {
	lib.Log("[%s] REGISTRAR unregistering peer %v", r.nodename, name)
	r.mutexConnections.Lock()
	defer r.mutexConnections.Unlock()
	if _, ok := r.connections[name]; ok {
		delete(r.connections, name)
		r.handleNodeDown(name)
		return
	}
}

// RegisterBehavior
func (r *registrar) RegisterBehavior(group, name string, behavior gen.ProcessBehavior, data interface{}) error {
	lib.Log("[%s] REGISTRAR registering behavior %q in group %q ", r.nodename, name, group)
	var groupBehaviors map[string]gen.RegisteredBehavior
	var exist bool

	r.mutexBehaviors.Lock()
	defer r.mutexBehaviors.Unlock()

	groupBehaviors, exist = r.behaviors[group]
	if !exist {
		groupBehaviors = make(map[string]gen.RegisteredBehavior)
		r.behaviors[group] = groupBehaviors
	}

	_, exist = groupBehaviors[name]
	if exist {
		return ErrTaken
	}

	rb := gen.RegisteredBehavior{
		Behavior: behavior,
		Data:     data,
	}
	groupBehaviors[name] = rb
	return nil
}

// RegisteredBehavior
func (r *registrar) RegisteredBehavior(group, name string) (gen.RegisteredBehavior, error) {
	var groupBehaviors map[string]gen.RegisteredBehavior
	var rb gen.RegisteredBehavior
	var exist bool

	r.mutexBehaviors.Lock()
	defer r.mutexBehaviors.Unlock()

	groupBehaviors, exist = r.behaviors[group]
	if !exist {
		return rb, ErrBehaviorGroupUnknown
	}

	rb, exist = groupBehaviors[name]
	if !exist {
		return rb, ErrBehaviorUnknown
	}
	return rb, nil
}

// RegisteredBehaviorGroup
func (r *registrar) RegisteredBehaviorGroup(group string) []gen.RegisteredBehavior {
	var groupBehaviors map[string]gen.RegisteredBehavior
	var exist bool
	var listrb []gen.RegisteredBehavior

	r.mutexBehaviors.Lock()
	defer r.mutexBehaviors.Unlock()

	groupBehaviors, exist = r.behaviors[group]
	if !exist {
		return listrb
	}

	for _, v := range groupBehaviors {
		listrb = append(listrb, v)
	}
	return listrb
}

// UnregisterBehavior
func (r *registrar) UnregisterBehavior(group, name string) error {
	lib.Log("[%s] REGISTRAR unregistering behavior %s in group %s ", r.nodename, name, group)
	var groupBehaviors map[string]gen.RegisteredBehavior
	var exist bool

	r.mutexBehaviors.Lock()
	defer r.mutexBehaviors.Unlock()

	groupBehaviors, exist = r.behaviors[group]
	if !exist {
		return ErrBehaviorUnknown
	}
	delete(groupBehaviors, name)

	// remove group if its empty
	if len(groupBehaviors) == 0 {
		delete(r.behaviors, group)
	}
	return nil
}

// ProcessInfo
func (r *registrar) ProcessInfo(pid etf.Pid) (gen.ProcessInfo, error) {
	p := r.ProcessByPid(pid)
	if p == nil {
		return gen.ProcessInfo{}, fmt.Errorf("undefined")
	}

	return p.Info(), nil
}

// ProcessByPid
func (r *registrar) ProcessByPid(pid etf.Pid) gen.Process {
	if p := r.getProcessByPid(pid); p != nil && p.IsAlive() {
		return p
	}
	// we must return nil explicitly, otherwise returning value is not nil
	// even for the nil(*process) due to the nature of interface type
	return nil
}

func (r *registrar) getProcessByPid(pid etf.Pid) *process {
	r.mutexProcesses.Lock()
	defer r.mutexProcesses.Unlock()
	if p, ok := r.processes[pid.ID]; ok {
		return p
	}
	// unknown process
	return nil
}

// ProcessByAlias
func (r *registrar) ProcessByAlias(alias etf.Alias) gen.Process {
	r.mutexAliases.Lock()
	defer r.mutexAliases.Unlock()
	if p, ok := r.aliases[alias]; ok && p.IsAlive() {
		return p
	}
	// unknown process
	return nil
}

// ProcessByName
func (r *registrar) ProcessByName(name string) gen.Process {
	var pid etf.Pid
	if name != "" {
		// requesting Process by name
		r.mutexNames.Lock()

		if p, ok := r.names[name]; ok {
			pid = p
		} else {
			r.mutexNames.Unlock()
			return nil
		}
		r.mutexNames.Unlock()
	}

	return r.ProcessByPid(pid)
}

// ProcessList
func (r *registrar) ProcessList() []gen.Process {
	list := []gen.Process{}
	r.mutexProcesses.Lock()
	for _, p := range r.processes {
		list = append(list, p)
	}
	r.mutexProcesses.Unlock()
	return list
}

// Nodes
func (r *registrar) Nodes() []string {
	list := []string{}
	for c := range r.connections {
		list = append(list, c.Name)
	}
	return list
}

//
// implementation of Router interface:
// RouteSend
// RouteSendReg
// RouteSendAlias
//

// RouteSend implements RouteSend method of Router interface
func (r *registrar) RouteSend(from etf.Pid, to etf.Pid, message etf.Term) error {
	// do not allow to send from the alien node. Proxy request must be used.
	if string(from.Node) != r.nodename {
		return ErrSenderUnknown
	}

	if string(to.Node) == r.nodename {
		if to.Creation != r.creation {
			// message is addressed to the previous incarnation of this PID
			return ErrProcessIncarnation
		}
		// local route
		r.mutexProcesses.Lock()
		p, exist := r.processes[to.ID]
		r.mutexProcesses.Unlock()
		if !exist {
			lib.Log("[%s] REGISTRAR route message by pid (local) %s failed. Unknown process", r.nodename, to)
			return ErrProcessUnknown
		}
		lib.Log("[%s] REGISTRAR route message by pid (local) %s", r.nodename, to)
		select {
		case p.mailBox <- gen.ProcessMailboxMessage{from, message}:
		default:
			return fmt.Errorf("WARNING! mailbox of %s is full. dropped message from %s", p.Self(), from)
		}
		return nil
	}

	// sending to remote node
	connection, err := r.getConnection(string(to.Name))
	if err != nil {
		return err
	}

	lib.Log("[%s] REGISTRAR route message by pid (remote) %s", r.nodename, to)
	return connection.Send(from, to, message)
}

// RouteSendReg implements RouteSendReg method of Router interface
func (r *registrar) RouteSendReg(from etf.Pid, to gen.ProcessID, message etf.Term) error {
	// do not allow to send from the alien node. Proxy request must be used.
	if string(from.Node) != r.nodename {
		return ErrSenderUnknown
	}

	if to.Node == r.nodename {
		// local route
		r.mutexNames.Lock()
		pid, ok := r.names[to.Name]
		r.mutexNames.Unlock()
		if !ok {
			lib.Log("[%s] REGISTRAR route message by gen.ProcessID (local) %s failed. Unknown process", r.nodename, to)
			return ErrProcessUnknown
		}
		lib.Log("[%s] REGISTRAR route message by gen.ProcessID (local) %s", r.nodename, to)
		return r.RouteSend(from, pid, message)
	}

	// send to remote node
	connection, err := r.getConnection(string(to.Name))
	if err != nil {
		return err
	}

	lib.Log("[%s] REGISTRAR route message by gen.ProcessID (remote) %s", r.nodename, to)
	return connection.SendReg(from, to, message)
}

// RouteSendAlias implements RouteSendAlias method of Router interface
func (r *registrar) RouteSendAlias(from etf.Pid, to etf.Alias, message etf.Term) error {
	// do not allow to send from the alien node. Proxy request must be used.
	if string(from.Node) != r.nodename {
		return ErrSenderUnknown
	}

	lib.Log("[%s] REGISTRAR route message by alias %s", r.nodename, to)
	if string(to.Node) == r.nodename {
		// local route by alias
		r.mutexAliases.Lock()
		process, ok := r.aliases[to]
		r.mutexAliases.Unlock()
		if !ok {
			lib.Log("[%s] REGISTRAR route message by alias (local) %s failed. Unknown process", r.nodename, to)
			return ErrProcessUnknown
		}
		return r.RouteSend(from, process.self, message)
	}

	// send to remote node
	connection, err := r.getConnection(string(to.Name))
	if err != nil {
		return err
	}

	lib.Log("[%s] REGISTRAR route message by alias (remote) %s", r.nodename, to)
	return connection.SendAlias(from, to, message)
}

func (r *registrar) getConnection(remoteNodeName string) (*Connection, error) {
	r.mutexConnections.Lock()
	connection, ok := r.connections[remoteNodeName]
	r.mutexConnections.Unlock()
	if ok {
		return connection, nil
	}

	if err := r.node.connect(remoteNodeName); err != nil {
		lib.Log("[%s] REGISTRAR no route to node %q: %s", r.nodename, remoteNodeName, err)
		return nil, ErrNoRoute
	}

	r.mutexConnections.Lock()
	connection, _ = r.connections[remoteNodeName]
	r.mutexConnection.Unlock()
	return connection, nil
}
