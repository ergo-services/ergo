package meta

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"ergo.services/ergo/gen"
)

const (
	defaultBufferSize int = 65535
)

//
// Port meta process
//

func CreatePort(options PortOptions) (gen.MetaBehavior, error) {
	if options.Cmd == "" {
		return nil, fmt.Errorf("empty options.Cmd")
	}

	// check sync.Pool
	if options.BufferPool != nil {
		b := options.BufferPool.Get()
		if _, ok := b.([]byte); ok == false {
			return nil, fmt.Errorf("options.BufferPool must be pool of []byte values")
		}
		// get it back to the pool
		options.BufferPool.Put(b)
	}

	if options.BufferSize < 1 {
		options.BufferSize = defaultBufferSize
	}

	p := &port{
		command:    options.Cmd,
		tag:        options.Tag,
		process:    options.Process,
		bufferPool: options.BufferPool,
		bufferSize: options.BufferSize,
	}
	return p, nil
}

type port struct {
	gen.MetaProcess
	tag        string
	process    gen.Atom
	bufferSize int
	bufferPool *sync.Pool
	bytesIn    uint64
	bytesOut   uint64
	command    string
	args       []string

	cmd *exec.Cmd
	in  io.WriteCloser
	out io.ReadCloser
}

func (p *port) Init(process gen.MetaProcess) error {
	p.MetaProcess = process
	return nil
}

func (p *port) Start() error {
	var buf []byte
	var to any

	id := p.ID()

	if p.process == "" {
		to = p.Parent()
	} else {
		to = p.process
	}

	cmd := exec.Command(p.command, p.args...)
	in, err := cmd.StdinPipe()
	if err != nil {
		p.Log().Error("unable to get stdin: %s", err)
		cmd.Process.Kill()
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		p.Log().Error("unable to get stdout: %s", err)
		cmd.Process.Kill()
		return err
	}
	p.in = in
	p.out = out

	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd = cmd

	defer func() {
		p.cmd.Process.Kill()
		message := MessagePortTerminated{
			ID:  id,
			Tag: p.tag,
		}
		if err := p.Send(to, message); err != nil {
			p.Log().Error("unable to send MessagePortTerminated to %s: %s", to, err)
			return
		}
	}()

	message := MessagePortStarted{
		ID:  id,
		Tag: p.tag,
	}
	if err := p.Send(to, message); err != nil {
		p.Log().Error("unable to send MessageTCPConnect to %v: %s", to, err)
		return err
	}

	for {
		if p.bufferPool == nil {
			buf = make([]byte, p.bufferSize)
		} else {
			buf = p.bufferPool.Get().([]byte)
		}

	retry:
		n, err := p.out.Read(buf)
		if err != nil {
			if n == 0 {
				// closed connection
				return nil
			}

			p.Log().Error("unable to read from stdin: %s", err)
			return err
		}
		if n == 0 {
			goto retry // use goto to get rid of buffer reallocation
		}
		message := MessagePort{
			ID:   id,
			Data: buf[:n],
		}
		atomic.AddUint64(&p.bytesIn, uint64(n))
		if err := p.Send(to, message); err != nil {
			p.Log().Error("unable to send MessagePort: %s", err)
			return err
		}
	}
}

func (p *port) HandleMessage(from gen.PID, message any) error {
	if p.MetaProcess != nil {
		p.Log().Error("ignored message from %s", from)
		return nil
	}
	switch m := message.(type) {
	case MessagePort:
		l := len(m.Data)
		lenD := l
		for {
			n, e := p.in.Write(m.Data[lenD-l:])
			if e != nil {
				return e
			}
			// check if something left
			l -= n
			if l == 0 {
				break
			}
		}
		atomic.AddUint64(&p.bytesOut, uint64(lenD))
		if p.bufferPool != nil {
			p.bufferPool.Put(m.Data)
		}
	default:
		p.Log().Error("unsupported message from %s. ignored", from)
	}
	return nil
}

func (p *port) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if p.MetaProcess != nil {
		p.Log().Error("ignored request from %s", from)
	}
	return nil, nil
}

func (p *port) Terminate(reason error) {
	// defer p.rw.Close()

	if reason == nil || reason == gen.TerminateReasonNormal {
		return
	}
	p.Log().Error("terminated abnormaly: %s", reason)
	if p.cmd != nil {
		p.in.Close()
		p.out.Close()
		p.cmd.Process.Kill()
	}
}

func (p *port) HandleInspect(from gen.PID, item ...string) map[string]string {
	return map[string]string{
		"tag":  p.tag,
		"cmd":  p.cmd.Args[0],
		"args": fmt.Sprint(p.cmd.Args[1:]),
		"pid":  fmt.Sprint(p.cmd.Process.Pid),
		"env":  fmt.Sprint(p.cmd.Env),
		"pwd":  p.cmd.Dir,
	}
}
