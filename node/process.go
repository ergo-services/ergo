package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

const (
	// DefaultProcessMailboxSize
	DefaultProcessMailboxSize = 100
)

var (
	syncReplyChannels = &sync.Pool{
		New: func() interface{} {
			return make(chan etf.Term, 2)
		},
	}

	directChannels = &sync.Pool{
		New: func() interface{} {
			return make(chan gen.ProcessDirectMessage, 1)
		},
	}
)

type process struct {
	coreInternal
	sync.RWMutex

	name     string
	self     etf.Pid
	behavior gen.ProcessBehavior
	env      map[gen.EnvKey]interface{}

	parent      *process
	groupLeader gen.Process
	aliases     []etf.Alias

	mailBox      chan gen.ProcessMailboxMessage
	gracefulExit chan gen.ProcessGracefulExitRequest
	direct       chan gen.ProcessDirectMessage

	context context.Context
	kill    context.CancelFunc
	exit    processExitFunc

	replyMutex sync.RWMutex
	reply      map[etf.Ref]chan etf.Term

	trapExit    bool
	compression Compression

	fallback gen.ProcessFallback
}

type processOptions struct {
	gen.ProcessOptions
	parent *process
}

type processExitFunc func(from etf.Pid, reason string) error

// Self
func (p *process) Self() etf.Pid {
	return p.self
}

// Name
func (p *process) Name() string {
	return p.name
}

// RegisterName
func (p *process) RegisterName(name string) error {
	if p.behavior == nil {
		return ErrProcessTerminated
	}
	return p.registerName(name, p.self)
}

// UnregisterName
func (p *process) UnregisterName(name string) error {
	if p.behavior == nil {
		return ErrProcessTerminated
	}
	prc := p.ProcessByName(name)
	if prc == nil {
		return ErrNameUnknown
	}
	if prc.Self() != p.self {
		return ErrNameOwner
	}
	return p.unregisterName(name)
}

// Kill
func (p *process) Kill() {
	if p.behavior == nil {
		return
	}
	p.kill()
}

// Exit
func (p *process) Exit(reason string) error {
	if p.behavior == nil {
		return ErrProcessTerminated
	}
	return p.exit(p.self, reason)
}

// Context
func (p *process) Context() context.Context {
	return p.context
}

// Parent
func (p *process) Parent() gen.Process {
	if p.parent == nil {
		return nil
	}
	return p.parent
}

// GroupLeader
func (p *process) GroupLeader() gen.Process {
	if p.groupLeader == nil {
		return nil
	}
	return p.groupLeader
}

// Links
func (p *process) Links() []etf.Pid {
	return p.processLinks(p.self)
}

// Monitors
func (p *process) Monitors() []etf.Pid {
	return p.processMonitors(p.self)
}

// MonitorsByName
func (p *process) MonitorsByName() []gen.ProcessID {
	return p.processMonitorsByName(p.self)
}

// MonitoredBy
func (p *process) MonitoredBy() []etf.Pid {
	return p.processMonitoredBy(p.self)
}

// Aliases
func (p *process) Aliases() []etf.Alias {
	return p.aliases
}

// Info
func (p *process) Info() gen.ProcessInfo {
	if p.behavior == nil {
		return gen.ProcessInfo{}
	}

	gl := p.self
	if p.groupLeader != nil {
		gl = p.groupLeader.Self()
	}
	links := p.Links()
	monitors := p.Monitors()
	monitorsByName := p.MonitorsByName()
	monitoredBy := p.MonitoredBy()
	return gen.ProcessInfo{
		PID:             p.self,
		Name:            p.name,
		GroupLeader:     gl,
		Links:           links,
		Monitors:        monitors,
		MonitorsByName:  monitorsByName,
		MonitoredBy:     monitoredBy,
		Aliases:         p.aliases,
		Status:          "running",
		MessageQueueLen: len(p.mailBox),
		TrapExit:        p.trapExit,
		Compression:     p.compression.Enable,
	}
}

// Send
func (p *process) Send(to interface{}, message etf.Term) error {
	if p.behavior == nil {
		return ErrProcessTerminated
	}
	switch receiver := to.(type) {
	case etf.Pid:
		return p.RouteSend(p.self, receiver, message)
	case string:
		return p.RouteSendReg(p.self, gen.ProcessID{Name: receiver, Node: string(p.self.Node)}, message)
	case etf.Atom:
		return p.RouteSendReg(p.self, gen.ProcessID{Name: string(receiver), Node: string(p.self.Node)}, message)
	case gen.ProcessID:
		return p.RouteSendReg(p.self, receiver, message)
	case etf.Alias:
		return p.RouteSendAlias(p.self, receiver, message)
	}
	return fmt.Errorf("Unknown receiver type")
}

// SendAfter
func (p *process) SendAfter(to interface{}, message etf.Term, after time.Duration) context.CancelFunc {
	//TODO: should we control the number of timers/goroutines have been created this way?
	ctx, cancel := context.WithCancel(p.context)
	go func() {
		// to prevent of timer leaks due to its not GCed until the timer fires
		timer := time.NewTimer(after)
		defer timer.Stop()
		defer cancel()

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if p.IsAlive() {
				p.Send(to, message)
			}
		}
	}()
	return cancel
}

// CreateAlias
func (p *process) CreateAlias() (etf.Alias, error) {
	if p.behavior == nil {
		return etf.Alias{}, ErrProcessTerminated
	}
	return p.newAlias(p)
}

// DeleteAlias
func (p *process) DeleteAlias(alias etf.Alias) error {
	if p.behavior == nil {
		return ErrProcessTerminated
	}
	return p.deleteAlias(p, alias)
}

// ListEnv
func (p *process) ListEnv() map[gen.EnvKey]interface{} {
	p.RLock()
	defer p.RUnlock()

	env := make(map[gen.EnvKey]interface{})

	if p.groupLeader != nil {
		for key, value := range p.groupLeader.ListEnv() {
			env[key] = value
		}
	}
	if p.parent != nil {
		for key, value := range p.parent.ListEnv() {
			env[key] = value
		}
	}
	for key, value := range p.env {
		env[key] = value
	}

	return env
}

// SetEnv
func (p *process) SetEnv(name gen.EnvKey, value interface{}) {
	p.Lock()
	defer p.Unlock()
	if value == nil {
		delete(p.env, name)
		return
	}
	p.env[name] = value
}

// Env
func (p *process) Env(name gen.EnvKey) interface{} {
	p.RLock()
	defer p.RUnlock()

	if value, ok := p.env[name]; ok {
		return value
	}

	if p.groupLeader != nil {
		return p.groupLeader.Env(name)
	}

	return nil
}

// Wait
func (p *process) Wait() {
	if p.IsAlive() {
		<-p.context.Done()
	}
}

// WaitWithTimeout
func (p *process) WaitWithTimeout(d time.Duration) error {
	if !p.IsAlive() {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return ErrTimeout
	case <-p.context.Done():
		return nil
	}
}

// Link
func (p *process) Link(with etf.Pid) error {
	if p.behavior == nil {
		return ErrProcessTerminated
	}
	return p.RouteLink(p.self, with)
}

// Unlink
func (p *process) Unlink(with etf.Pid) error {
	if p.behavior == nil {
		return ErrProcessTerminated
	}
	return p.RouteUnlink(p.self, with)
}

// IsAlive
func (p *process) IsAlive() bool {
	if p.behavior == nil {
		return false
	}
	return p.context.Err() == nil
}

// NodeName
func (p *process) NodeName() string {
	return p.coreNodeName()
}

// NodeStop
func (p *process) NodeStop() {
	p.coreStop()
}

// NodeUptime
func (p *process) NodeUptime() int64 {
	return p.coreUptime()
}

// Children
func (p *process) Children() ([]etf.Pid, error) {
	c, err := p.directRequest(gen.MessageDirectChildren{}, 5)
	if err != nil {
		return []etf.Pid{}, err
	}
	children, correct := c.([]etf.Pid)
	if correct == false {
		return []etf.Pid{}, err
	}
	return children, nil
}

// SetTrapExit
func (p *process) SetTrapExit(trap bool) {
	p.trapExit = trap
}

// TrapExit
func (p *process) TrapExit() bool {
	return p.trapExit
}

// SetCompression
func (p *process) SetCompression(enable bool) {
	p.compression.Enable = enable
}

// Compression
func (p *process) Compression() bool {
	return p.compression.Enable
}

// CompressionLevel
func (p *process) CompressionLevel() int {
	return p.compression.Level
}

// SetCompressionLevel
func (p *process) SetCompressionLevel(level int) bool {
	if level < 1 || level > 9 {
		return false
	}
	p.compression.Level = level
	return true
}

// CompressionThreshold
func (p *process) CompressionThreshold() int {
	return p.compression.Threshold
}

// SetCompressionThreshold
func (p *process) SetCompressionThreshold(threshold int) bool {
	if threshold < DefaultCompressionThreshold {
		return false
	}
	p.compression.Threshold = threshold
	return true
}

// Behavior
func (p *process) Behavior() gen.ProcessBehavior {
	p.Lock()
	defer p.Unlock()
	if p.behavior == nil {
		return nil
	}
	return p.behavior
}

// Direct
func (p *process) Direct(request interface{}) (interface{}, error) {
	return p.directRequest(request, gen.DefaultCallTimeout)
}

// DirectWithTimeout
func (p *process) DirectWithTimeout(request interface{}, timeout int) (interface{}, error) {
	if timeout < 1 {
		timeout = 5
	}
	return p.directRequest(request, timeout)
}

// MonitorNode
func (p *process) MonitorNode(name string) etf.Ref {
	ref := p.MakeRef()
	p.monitorNode(p.self, name, ref)
	return ref
}

// DemonitorNode
func (p *process) DemonitorNode(ref etf.Ref) bool {
	return p.demonitorNode(ref)
}

// MonitorProcess
func (p *process) MonitorProcess(process interface{}) etf.Ref {
	ref := p.MakeRef()
	switch mp := process.(type) {
	case etf.Pid:
		p.RouteMonitor(p.self, mp, ref)
		return ref
	case gen.ProcessID:
		p.RouteMonitorReg(p.self, mp, ref)
		return ref
	case string:
		p.RouteMonitorReg(p.self, gen.ProcessID{Name: mp, Node: string(p.self.Node)}, ref)
		return ref
	case etf.Atom:
		p.RouteMonitorReg(p.self, gen.ProcessID{Name: string(mp), Node: string(p.self.Node)}, ref)
		return ref
	}

	// create fake gen.ProcessID. Monitor will send MessageDown with "noproc" as a reason
	p.RouteMonitorReg(p.self, gen.ProcessID{Node: string(p.self.Node)}, ref)
	return ref
}

// DemonitorProcess
func (p *process) DemonitorProcess(ref etf.Ref) bool {
	if err := p.RouteDemonitor(p.self, ref); err != nil {
		return false
	}
	return true
}

// RemoteSpawn makes request to spawn new process on a remote node
func (p *process) RemoteSpawn(node string, object string, opts gen.RemoteSpawnOptions, args ...etf.Term) (etf.Pid, error) {
	return p.RemoteSpawnWithTimeout(gen.DefaultCallTimeout, node, object, opts, args...)
}

// RemoteSpawnWithTimeout makes request to spawn new process on a remote node with given timeout
func (p *process) RemoteSpawnWithTimeout(timeout int, node string, object string, opts gen.RemoteSpawnOptions, args ...etf.Term) (etf.Pid, error) {
	ref := p.MakeRef()
	p.PutSyncRequest(ref)
	request := gen.RemoteSpawnRequest{
		From:    p.self,
		Ref:     ref,
		Options: opts,
	}
	if err := p.RouteSpawnRequest(node, object, request, args...); err != nil {
		p.CancelSyncRequest(ref)
		return etf.Pid{}, err
	}

	reply, err := p.WaitSyncReply(ref, timeout)
	if err != nil {
		return etf.Pid{}, err
	}

	// Result of the operation. If Result is a process identifier,
	// the operation succeeded and the process identifier is the
	// identifier of the newly created process. If Result is an atom,
	// the operation failed and the atom identifies failure reason.
	switch r := reply.(type) {
	case etf.Pid:
		m := etf.Ref{} // empty reference
		if opts.Monitor != m {
			p.RouteMonitor(p.self, r, opts.Monitor)
		}
		if opts.Link {
			p.RouteLink(p.self, r)
		}
		return r, nil
	case etf.Atom:
		switch string(r) {
		case ErrTaken.Error():
			return etf.Pid{}, ErrTaken
		case ErrBehaviorUnknown.Error():
			return etf.Pid{}, ErrBehaviorUnknown
		}
		return etf.Pid{}, fmt.Errorf(string(r))
	}

	return etf.Pid{}, fmt.Errorf("unknown result: %#v", reply)
}

// Spawn
func (p *process) Spawn(name string, opts gen.ProcessOptions, behavior gen.ProcessBehavior, args ...etf.Term) (gen.Process, error) {
	options := processOptions{
		ProcessOptions: opts,
		parent:         p,
	}
	return p.spawn(name, options, behavior, args...)
}

func (p *process) directRequest(request interface{}, timeout int) (interface{}, error) {
	if p.direct == nil {
		return nil, ErrProcessTerminated
	}

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)

	direct := gen.ProcessDirectMessage{
		Message: request,
		Reply:   directChannels.Get().(chan gen.ProcessDirectMessage),
	}

	// sending request
	select {
	case p.direct <- direct:
		timer.Reset(time.Second * time.Duration(timeout))
	case <-timer.C:
		return nil, ErrProcessBusy
	}

	// receiving response
	select {
	case response := <-direct.Reply:
		directChannels.Put(direct.Reply)
		if response.Err != nil {
			return nil, response.Err
		}

		return response.Message, nil
	case <-timer.C:
		return nil, ErrTimeout
	}
}

// PutSyncRequest
func (p *process) PutSyncRequest(ref etf.Ref) {
	if p.reply == nil {
		return
	}
	reply := syncReplyChannels.Get().(chan etf.Term)
	p.replyMutex.Lock()
	p.reply[ref] = reply
	p.replyMutex.Unlock()
}

// PutSyncReply
func (p *process) PutSyncReply(ref etf.Ref, reply etf.Term) error {
	if p.reply == nil {
		return ErrProcessTerminated
	}

	p.replyMutex.RLock()
	rep, ok := p.reply[ref]
	p.replyMutex.RUnlock()

	if !ok {
		// ignore this reply, no process waiting for it
		return nil
	}
	select {
	case rep <- reply:
	}
	return nil
}

// CancelSyncRequest
func (p *process) CancelSyncRequest(ref etf.Ref) {
	p.replyMutex.Lock()
	delete(p.reply, ref)
	p.replyMutex.Unlock()
}

// WaitSyncReply
func (p *process) WaitSyncReply(ref etf.Ref, timeout int) (etf.Term, error) {
	if p.reply == nil {
		return nil, ErrProcessTerminated
	}

	p.replyMutex.RLock()
	reply, wait_for_reply := p.reply[ref]
	p.replyMutex.RUnlock()

	if !wait_for_reply {
		return nil, fmt.Errorf("Unknown request")
	}

	defer func(ref etf.Ref) {
		p.replyMutex.Lock()
		delete(p.reply, ref)
		p.replyMutex.Unlock()
	}(ref)

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)
	timer.Reset(time.Second * time.Duration(timeout))

	for {
		select {
		case m := <-reply:
			syncReplyChannels.Put(reply)
			return m, nil
		case <-timer.C:
			return nil, ErrTimeout
		case <-p.context.Done():
			return nil, ErrProcessTerminated
		}
	}

}

// ProcessChannels
func (p *process) ProcessChannels() gen.ProcessChannels {
	return gen.ProcessChannels{
		Mailbox:      p.mailBox,
		Direct:       p.direct,
		GracefulExit: p.gracefulExit,
	}
}
