package gen

type TargetManager interface {
	HasLink(consumer PID, target any) bool
	HasMonitor(consumer PID, target any) bool

	LinkPID(consumer PID, target PID) error
	UnlinkPID(consumer PID, target PID) error
	MonitorPID(consumer PID, target PID) error
	DemonitorPID(consumer PID, target PID) error

	LinkProcessID(consumer PID, target ProcessID) error
	UnlinkProcessID(consumer PID, target ProcessID) error
	MonitorProcessID(consumer PID, target ProcessID) error
	DemonitorProcessID(consumer PID, target ProcessID) error

	LinkAlias(consumer PID, target Alias) error
	UnlinkAlias(consumer PID, target Alias) error
	MonitorAlias(consumer PID, target Alias) error
	DemonitorAlias(consumer PID, target Alias) error

	LinkNode(consumer PID, target Atom) error
	UnlinkNode(consumer PID, target Atom) error
	MonitorNode(consumer PID, target Atom) error
	DemonitorNode(consumer PID, target Atom) error

	LinkEvent(consumer PID, event Event) (lastEvents []MessageEvent, err error)
	UnlinkEvent(consumer PID, event Event) error
	MonitorEvent(consumer PID, event Event) (lastEvents []MessageEvent, err error)
	DemonitorEvent(consumer PID, event Event) error

	RegisterEvent(producer PID, name Atom, options EventOptions) (Ref, error)
	UnregisterEvent(producer PID, name Atom) error
	PublishEvent(from PID, token Ref, options MessageOptions, message MessageEvent) error
	EventInfo(event Event) (EventInfo, error)
	EventRangeInfo(fn func(EventInfo) bool) error
	EventListInfo(timestamp int64, limit int, filter ...func(EventInfo) bool) ([]EventInfo, error)

	LinksFor(consumer PID) []any
	MonitorsFor(consumer PID) []any
	EventsFor(producer PID) []Event

	TerminatedTargetNode(node Atom, reason error)
	TerminatedTargetPID(pid PID, reason error)
	TerminatedTargetProcessID(processID ProcessID, reason error)
	TerminatedTargetAlias(alias Alias, reason error)
	TerminatedTargetEvent(event Event, reason error)

	// local process terminated:
	// can be either target or consumer, or both
	TerminatedProcess(pid PID, reason error)

	Info() TargetManagerInfo
}

// EventInfo contains event metadata and statistics
type EventInfo struct {
	CreatedAt          int64
	Event              Event
	Producer           PID
	BufferSize         int
	CurrentBuffer      int
	Notify             bool
	Open               bool
	Subscribers        int64
	MessagesPublished  int64
	MessagesLocalSent  int64
	MessagesRemoteSent int64
	// LastPublishedAt is the unix-nanos timestamp of the most recent SendEvent on this event,
	// or 0 if nothing has ever been published. Useful for detecting silent producers: a
	// non-zero MessagesPublished combined with a stale LastPublishedAt means the producer
	// has stopped publishing.
	LastPublishedAt int64
}

type TargetManagerInfo struct {
	Links    int64
	Monitors int64
	Events   int64

	// Statistics
	ExitSignalsProduced   int64 // Total exit signals generated
	ExitSignalsDelivered  int64 // Total exit signals delivered by dispatchers
	DownMessagesProduced  int64 // Total down messages generated
	DownMessagesDelivered int64 // Total down messages delivered
	EventsPublished       int64 // Total events published by local producers
	EventsReceived        int64 // Total events received from remote nodes
	EventsLocalSent       int64 // Total event messages sent to local subscribers
	EventsRemoteSent      int64 // Total event messages sent to remote subscribers
}
