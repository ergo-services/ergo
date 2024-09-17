package meta

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
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
	if options.ReadBufferPool != nil {
		b := options.ReadBufferPool.Get()
		if _, ok := b.([]byte); ok == false {
			return nil, fmt.Errorf("options.BufferPool must be pool of []byte values")
		}
		// get it back to the pool
		options.ReadBufferPool.Put(b)
	}

	if options.ReadBufferSize < 1 {
		options.ReadBufferSize = defaultBufferSize
	}

	if options.WriteBufferKeepAlive != nil {
		if options.WriteBufferKeepAlivePeriod == 0 {
			return nil, fmt.Errorf("enabled KeepAlive options with zero Period")
		}
	}

	p := &port{
		command:        options.Cmd,
		args:           options.Args,
		tag:            options.Tag,
		process:        options.Process,
		readBufferPool: options.ReadBufferPool,
		readBufferSize: options.ReadBufferSize,
		writeBuffer:    options.WriteBuffer,
	}
	return p, nil
}

type port struct {
	gen.MetaProcess
	tag            string
	process        gen.Atom
	readBufferSize int
	readBufferPool *sync.Pool

	writeBuffer                bool
	writeBufferKeepAlive       []byte
	writeBufferKeepAlivePeriod time.Duration

	bytesIn  uint64
	bytesOut uint64
	command  string
	args     []string

	cmd    *exec.Cmd
	in     io.Writer
	out    io.ReadCloser
	errout io.ReadCloser
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

	errout, err := cmd.StderrPipe()
	if err != nil {
		p.Log().Error("unable to get stderr: %s", err)
		cmd.Process.Kill()
		return err
	}

	p.in = in
	p.out = out
	p.errout = errout

	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd = cmd

	if p.writeBuffer {
		if p.writeBufferKeepAlive != nil {
			p.in = lib.NewFlusherWithKeepAlive(p.in, p.writeBufferKeepAlive, p.writeBufferKeepAlivePeriod)
		} else {
			p.in = lib.NewFlusher(p.in)
		}
	}

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
		p.Log().Error("unable to send MessagePortStarted to %v: %s", to, err)
		return err
	}
	for {
		if p.readBufferPool == nil {
			buf = make([]byte, p.readBufferSize)
		} else {
			buf = p.readBufferPool.Get().([]byte)
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
		if p.readBufferPool != nil {
			p.readBufferPool.Put(m.Data)
		}
	default:
		p.Log().Error("unsupported message type '%T' from %s. ignored", message, from)
	}
	return nil
}

func (p *port) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (p *port) Terminate(reason error) {
	if reason == nil || reason == gen.TerminateReasonNormal {
		return
	}
	p.Log().Error("terminated abnormaly: %s", reason)
	if p.cmd != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}
}

func (p *port) HandleInspect(from gen.PID, item ...string) map[string]string {
	return map[string]string{
		"tag":      p.tag,
		"cmd":      p.cmd.Args[0],
		"args":     fmt.Sprint(p.cmd.Args[1:]),
		"pid":      fmt.Sprint(p.cmd.Process.Pid),
		"env":      fmt.Sprint(p.cmd.Env),
		"pwd":      p.cmd.Dir,
		"bytesIn":  fmt.Sprintf("%d", p.bytesIn),
		"bytesOut": fmt.Sprintf("%d", p.bytesOut),
	}
}
