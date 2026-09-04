package gen

import (
	"sync"
)

type MailboxMessageType int

const (
	MailboxMessageTypeRegular MailboxMessageType = 0
	MailboxMessageTypeRequest MailboxMessageType = 1
	MailboxMessageTypeEvent   MailboxMessageType = 2
	MailboxMessageTypeSpan    MailboxMessageType = 3

	MailboxMessageTypeExit    MailboxMessageType = 10
	MailboxMessageTypeInspect MailboxMessageType = 11
)

type MailboxMessage struct {
	From    PID
	Ref     Ref
	Type    MailboxMessageType
	Target  any
	Message any
	Tracing Tracing
}

var (
	mbm = &sync.Pool{
		New: func() any {
			return &MailboxMessage{}
		},
	}
)

func TakeMailboxMessage() *MailboxMessage {
	return mbm.Get().(*MailboxMessage)
}

func ReleaseMailboxMessage(m *MailboxMessage) {
	var emptyPID PID
	var emptyRef Ref
	m.Message = nil
	m.Target = nil
	m.Type = 0
	m.From = emptyPID
	m.Ref = emptyRef
	m.Tracing = Tracing{}
	mbm.Put(m)
}
