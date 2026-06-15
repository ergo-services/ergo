package unit

import "ergo.services/ergo/gen"

// processOverrides holds an optional handler per non-egress gen.Process method.
// When a field is set, the corresponding mockProcess method returns the handler's
// result instead of the default. Egress methods (Send/Call/Spawn/Link/...) are
// controlled via the typed stub sugar on the Subject instead.
//
// The On<Method> setters live on *Subject: configure them as sub.On<Method>(...)
// after spawn (used by subsequent deliveries).
type processOverrides struct {
	node         func() gen.Node
	name         func() gen.Atom
	pid          func() gen.PID
	leader       func() gen.PID
	parent       func() gen.PID
	application  func() gen.Application
	uptime       func() int64
	state        func() gen.ProcessState
	behavior     func() gen.ProcessBehavior
	behaviorName func() string
	mailbox      func() gen.ProcessMailbox
	log          func() gen.Log
	aliases      func() []gen.Alias
	events       func() []gen.Atom

	envList    func() map[gen.Env]any
	setEnv     func(name gen.Env, value any)
	env        func(name gen.Env) (any, bool)
	envDefault func(name gen.Env, def any) any

	compression             func() bool
	setCompression          func(enabled bool) error
	compressionType         func() gen.CompressionType
	setCompressionType      func(ctype gen.CompressionType) error
	compressionLevel        func() gen.CompressionLevel
	setCompressionLevel     func(level gen.CompressionLevel) error
	compressionThreshold    func() int
	setCompressionThreshold func(threshold int) error
	sendPriority            func() gen.MessagePriority
	setSendPriority         func(priority gen.MessagePriority) error
	setProcessKind          func(kind gen.ProcessKind) error
	setKeepNetworkOrder     func(order bool) error
	keepNetworkOrder        func() bool
	setImportantDelivery    func(important bool) error
	importantDelivery       func() bool
	setTracingSampler       func(sampler gen.TracingSampler) error
	tracingSampler          func() gen.TracingSampler
	registerName            func(name gen.Atom) error
	unregisterName          func() error

	deleteAlias     func(alias gen.Alias) error
	unregisterEvent func(name gen.Atom) error

	inspect     func(target gen.PID, item ...string) (map[string]string, error)
	inspectMeta func(meta gen.Alias, item ...string) (map[string]string, error)
	info        func() (gen.ProcessInfo, error)
	metaInfo    func(meta gen.Alias) (gen.MetaInfo, error)

	propagatingTrace           func() gen.Tracing
	setPropagatingTrace        func(t gen.Tracing)
	setTracingAttribute        func(key, value string)
	removeTracingAttribute     func(key string)
	setTracingSpanAttribute    func(key, value string)
	tracingAttributes          func() []gen.TracingAttribute
	clearTracingSpanAttributes func()
	sendTracingSpan            func(span gen.TracingSpan)
}

// On<Method> setters for the process under test. Configure after spawn; the next
// delivery uses them.

func (s *Subject) OnNode(fn func() gen.Node)                { s.process.ov.node = fn }
func (s *Subject) OnName(fn func() gen.Atom)                { s.process.ov.name = fn }
func (s *Subject) OnPID(fn func() gen.PID)                  { s.process.ov.pid = fn }
func (s *Subject) OnLeader(fn func() gen.PID)               { s.process.ov.leader = fn }
func (s *Subject) OnParent(fn func() gen.PID)               { s.process.ov.parent = fn }
func (s *Subject) OnApplication(fn func() gen.Application)  { s.process.ov.application = fn }
func (s *Subject) OnUptime(fn func() int64)                 { s.process.ov.uptime = fn }
func (s *Subject) OnState(fn func() gen.ProcessState)       { s.process.ov.state = fn }
func (s *Subject) OnBehavior(fn func() gen.ProcessBehavior) { s.process.ov.behavior = fn }
func (s *Subject) OnBehaviorName(fn func() string)          { s.process.ov.behaviorName = fn }
func (s *Subject) OnMailbox(fn func() gen.ProcessMailbox)   { s.process.ov.mailbox = fn }
func (s *Subject) OnLog(fn func() gen.Log)                  { s.process.ov.log = fn }
func (s *Subject) OnAliases(fn func() []gen.Alias)          { s.process.ov.aliases = fn }
func (s *Subject) OnEvents(fn func() []gen.Atom)            { s.process.ov.events = fn }

func (s *Subject) OnEnvList(fn func() map[gen.Env]any)             { s.process.ov.envList = fn }
func (s *Subject) OnSetEnv(fn func(name gen.Env, value any))       { s.process.ov.setEnv = fn }
func (s *Subject) OnEnv(fn func(name gen.Env) (any, bool))         { s.process.ov.env = fn }
func (s *Subject) OnEnvDefault(fn func(name gen.Env, def any) any) { s.process.ov.envDefault = fn }

func (s *Subject) OnCompression(fn func() bool)                    { s.process.ov.compression = fn }
func (s *Subject) OnSetCompression(fn func(enabled bool) error)    { s.process.ov.setCompression = fn }
func (s *Subject) OnCompressionType(fn func() gen.CompressionType) { s.process.ov.compressionType = fn }
func (s *Subject) OnSetCompressionType(fn func(ctype gen.CompressionType) error) {
	s.process.ov.setCompressionType = fn
}
func (s *Subject) OnCompressionLevel(fn func() gen.CompressionLevel) {
	s.process.ov.compressionLevel = fn
}
func (s *Subject) OnSetCompressionLevel(fn func(level gen.CompressionLevel) error) {
	s.process.ov.setCompressionLevel = fn
}
func (s *Subject) OnCompressionThreshold(fn func() int) { s.process.ov.compressionThreshold = fn }
func (s *Subject) OnSetCompressionThreshold(fn func(threshold int) error) {
	s.process.ov.setCompressionThreshold = fn
}
func (s *Subject) OnSendPriority(fn func() gen.MessagePriority) { s.process.ov.sendPriority = fn }
func (s *Subject) OnSetSendPriority(fn func(priority gen.MessagePriority) error) {
	s.process.ov.setSendPriority = fn
}
func (s *Subject) OnSetProcessKind(fn func(kind gen.ProcessKind) error) {
	s.process.ov.setProcessKind = fn
}
func (s *Subject) OnSetKeepNetworkOrder(fn func(order bool) error) {
	s.process.ov.setKeepNetworkOrder = fn
}
func (s *Subject) OnKeepNetworkOrder(fn func() bool) { s.process.ov.keepNetworkOrder = fn }
func (s *Subject) OnSetImportantDelivery(fn func(important bool) error) {
	s.process.ov.setImportantDelivery = fn
}
func (s *Subject) OnImportantDelivery(fn func() bool) { s.process.ov.importantDelivery = fn }
func (s *Subject) OnSetTracingSampler(fn func(sampler gen.TracingSampler) error) {
	s.process.ov.setTracingSampler = fn
}
func (s *Subject) OnTracingSampler(fn func() gen.TracingSampler) { s.process.ov.tracingSampler = fn }
func (s *Subject) OnRegisterName(fn func(name gen.Atom) error)   { s.process.ov.registerName = fn }
func (s *Subject) OnUnregisterName(fn func() error)              { s.process.ov.unregisterName = fn }

func (s *Subject) OnDeleteAlias(fn func(alias gen.Alias) error)   { s.process.ov.deleteAlias = fn }
func (s *Subject) OnUnregisterEvent(fn func(name gen.Atom) error) { s.process.ov.unregisterEvent = fn }

func (s *Subject) OnInspect(fn func(target gen.PID, item ...string) (map[string]string, error)) {
	s.process.ov.inspect = fn
}
func (s *Subject) OnInspectMeta(fn func(meta gen.Alias, item ...string) (map[string]string, error)) {
	s.process.ov.inspectMeta = fn
}
func (s *Subject) OnInfo(fn func() (gen.ProcessInfo, error)) { s.process.ov.info = fn }
func (s *Subject) OnMetaInfo(fn func(meta gen.Alias) (gen.MetaInfo, error)) {
	s.process.ov.metaInfo = fn
}

func (s *Subject) OnPropagatingTrace(fn func() gen.Tracing) { s.process.ov.propagatingTrace = fn }
func (s *Subject) OnSetPropagatingTrace(fn func(t gen.Tracing)) {
	s.process.ov.setPropagatingTrace = fn
}
func (s *Subject) OnSetTracingAttribute(fn func(key, value string)) {
	s.process.ov.setTracingAttribute = fn
}
func (s *Subject) OnRemoveTracingAttribute(fn func(key string)) {
	s.process.ov.removeTracingAttribute = fn
}
func (s *Subject) OnSetTracingSpanAttribute(fn func(key, value string)) {
	s.process.ov.setTracingSpanAttribute = fn
}
func (s *Subject) OnTracingAttributes(fn func() []gen.TracingAttribute) {
	s.process.ov.tracingAttributes = fn
}
func (s *Subject) OnClearTracingSpanAttributes(fn func()) {
	s.process.ov.clearTracingSpanAttributes = fn
}
func (s *Subject) OnSendTracingSpan(fn func(span gen.TracingSpan)) { s.process.ov.sendTracingSpan = fn }
