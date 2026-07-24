package meta

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync/atomic"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

const (
	defaultBufferSize int = 8192
)

//
// Port meta process
//

func CreatePort(options PortOptions) (gen.MetaBehavior, error) {
	if options.Cmd == "" {
		return nil, fmt.Errorf("empty Cmd option")
	}

	p := &port{
		options: options,
	}

	if options.Binary.Enable == false {
		return p, nil
	}

	// check sync.Pool
	if p.options.Binary.ReadBufferPool != nil {
		b := p.options.Binary.ReadBufferPool.Get()
		if _, ok := b.([]byte); ok == false {
			return nil, fmt.Errorf("ReadBufferPool must be a pool of []byte values")
		}
		// get it back to the pool
		p.options.Binary.ReadBufferPool.Put(b)
	}

	if p.options.Binary.ReadBufferSize < 1 {
		p.options.Binary.ReadBufferSize = defaultBufferSize
	}

	if len(p.options.Binary.WriteBufferKeepAlive) > 0 {
		// enabled keepalive message
		if p.options.Binary.WriteBufferKeepAlivePeriod == 0 {
			return nil, fmt.Errorf("enabled WriteBufferKeepAlive options with zero Period")
		}
	}

	if err := p.options.Binary.ReadChunk.IsValid(); err != nil {
		return nil, err
	}

	return p, nil
}

// portWriter adapts a flushing io.Writer to io.WriteCloser for the stdin pipe.
type portWriter struct {
	io.Writer
	io.Closer
}

func (w *portWriter) Close() error {
	if f, ok := w.Writer.(lib.Flusher); ok {
		f.Stop()
	}
	return w.Closer.Close()
}

type port struct {
	gen.MetaProcess
	cmd *exec.Cmd

	options PortOptions

	bytesIn  uint64
	bytesOut uint64

	in     io.WriteCloser
	out    io.ReadCloser
	errout io.ReadCloser
}

func (p *port) Init(process gen.MetaProcess) error {
	var err error

	p.MetaProcess = process
	p.cmd = exec.Command(p.options.Cmd, p.options.Args...)

	if p.in, err = p.cmd.StdinPipe(); err != nil {
		p.Log().Error("unable to get stdin: %s", err)
		return err
	}

	if p.out, err = p.cmd.StdoutPipe(); err != nil {
		p.Log().Error("unable to get stdout: %s", err)
		p.in.Close()
		return err
	}

	if p.errout, err = p.cmd.StderrPipe(); err != nil {
		p.Log().Error("unable to get stderr: %s", err)
		p.out.Close()
		p.in.Close()
		return err
	}

	// from the OS
	if p.options.EnableEnvOS == true {
		p.cmd.Env = os.Environ()
	}

	// from the meta process
	if p.options.EnableEnvMeta == true {
		env := []string{}
		for n, v := range p.EnvList() {
			env = append(env, fmt.Sprintf("%s=%v", n, v))
		}
		p.cmd.Env = append(p.cmd.Env, env...)
	}

	// from options
	env := []string{}
	for n, v := range p.options.Env {
		env = append(env, fmt.Sprintf("%s=%v", n, v))
	}
	p.cmd.Env = append(p.cmd.Env, env...)

	// binary mode: wrap the stdin writer with a flusher here, in the synchronous
	// setup, so it is final before the start/handle goroutines exist and a
	// HandleMessage write can never race this assignment.
	if p.options.Binary.Enable {
		sc := &portWriter{Closer: p.in}
		if len(p.options.Binary.WriteBufferKeepAlive) > 0 {
			sc.Writer = lib.NewFlusherWithKeepAlive(p.in,
				p.options.Binary.WriteBufferKeepAlive,
				p.options.Binary.WriteBufferKeepAlivePeriod)
		} else {
			sc.Writer = lib.NewFlusher(p.in)
		}
		p.in = sc
	}

	return nil
}

func (p *port) Start() error {
	var to any

	if err := p.cmd.Start(); err != nil {
		p.out.Close()
		p.in.Close()
		p.errout.Close()
		return err
	}

	if p.options.Process == "" {
		to = p.Parent()
	} else {
		to = p.options.Process
	}

	defer func() {
		p.cmd.Process.Kill()
		message := MessagePortTerminate{
			ID:  p.ID(),
			Tag: p.options.Tag,
		}
		if err := p.Send(to, message); err != nil {
			// gen.ErrNotAllowed means parent process was terminated
			p.Log().Trace("unable to send MessagePortTerminate to %s: %s", to, err)
			return
		}
	}()

	message := MessagePortStart{
		ID:  p.ID(),
		Tag: p.options.Tag,
	}
	if err := p.Send(to, message); err != nil {
		p.Log().Error("unable to send MessagePortStart to %v: %s", to, err)
		return err
	}

	// run stderr reader
	go p.readStderr(to)

	if p.options.Binary.Enable == false {
		return p.readStdoutText(to)
	}

	//
	// binary mode (the stdin writer was wrapped in Init)
	//

	if p.options.Binary.ReadChunk.Enable == false {
		return p.readStdoutData(to)
	}

	return p.readStdoutDataChunk(to)
}

func (p *port) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case MessagePortText:
		data := []byte(m.Text)
		l := len(m.Text)
		lenD := l
		for {
			n, e := p.in.Write(data[lenD-l:])
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

	case MessagePortData:
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
		if p.options.Binary.ReadBufferPool != nil {
			p.options.Binary.ReadBufferPool.Put(m.Data)
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
	if p.cmd != nil {
		p.in.Close()
		p.out.Close()
		p.errout.Close()
		if p.cmd.Process != nil {
			p.cmd.Process.Kill()
		}
		p.cmd.Wait()
	}

	if reason == nil {
		return
	}

	if reason == gen.TerminateReasonShutdown || reason == gen.TerminateReasonNormal {
		return
	}

	p.Log().Error("terminated abnormaly: %s", reason)
}

func (p *port) HandleInspect(from gen.PID, item ...string) map[string]string {
	return map[string]string{
		"tag":               p.options.Tag,
		"cmd":               p.cmd.Args[0],
		"binary":            fmt.Sprint(p.options.Binary.Enable),
		"binary.read_chunk": fmt.Sprint(p.options.Binary.ReadChunk.Enable),
		"args":              fmt.Sprint(p.cmd.Args[1:]),
		"pid":               fmt.Sprint(p.cmd.Process.Pid),
		"env":               fmt.Sprint(p.cmd.Env),
		"pwd":               p.cmd.Dir,
		"bytesIn":           fmt.Sprintf("%d", p.bytesIn),
		"bytesOut":          fmt.Sprintf("%d", p.bytesOut),
	}
}

func (p *port) readStdoutData(to any) error {
	var buf []byte

	id := p.ID()

	for {
		if p.options.Binary.ReadBufferPool == nil {
			buf = make([]byte, p.options.Binary.ReadBufferSize)
		} else {
			buf = p.options.Binary.ReadBufferPool.Get().([]byte)
			if len(buf) == 0 {
				if cap(buf) == 0 {
					buf = make([]byte, p.options.Binary.ReadBufferSize)
				} else {
					buf = buf[0:cap(buf)]
				}
			}
		}

	next:
		n, err := p.out.Read(buf)
		if err != nil {
			if n == 0 {
				// closed stdin
				return nil
			}

			p.Log().Error("unable to read from stdin: %s", err)
			return err
		}

		if n == 0 {
			goto next
		}

		atomic.AddUint64(&p.bytesIn, uint64(n))

		message := MessagePortData{
			ID:   id,
			Tag:  p.options.Tag,
			Data: buf[:n],
		}

		if err := p.Send(to, message); err != nil {
			p.Log().Error("unable to send MessagePort: %s", err)
			return err
		}

		if p.options.Binary.ReadBufferPool == nil {
			buf = make([]byte, p.options.Binary.ReadBufferSize)
			continue
		}

		buf = p.options.Binary.ReadBufferPool.Get().([]byte)
	}
}

func (p *port) readStdoutDataChunk(to any) error {
	id := p.ID()
	err := readChunks(countReader{p.out, &p.bytesIn}, p.options.Binary.ReadChunk,
		p.options.Binary.ReadBufferSize, p.options.Binary.ReadBufferPool,
		func(chunk []byte) error {
			return p.Send(to, MessagePortData{ID: id, Tag: p.options.Tag, Data: chunk})
		})
	if err != nil {
		p.Log().Error("unable to read binary chunk from stdin: %s", err)
	}
	return err
}

func (p *port) readStdoutText(to any) error {
	id := p.ID()
	out := bufio.NewScanner(p.out)
	if p.options.SplitFuncStdout != nil {
		out.Split(p.options.SplitFuncStdout)
	}

	for out.Scan() {
		txt := out.Text()
		message := MessagePortText{
			ID:   id,
			Tag:  p.options.Tag,
			Text: txt,
		}
		atomic.AddUint64(&p.bytesIn, uint64(len(txt)))
		if err := p.Send(to, message); err != nil {
			p.Log().Error("unable to send MessagePortError: %s", err)
			return err
		}
	}

	return out.Err()
}

func (p *port) readStderr(to any) {
	id := p.ID()
	out := bufio.NewScanner(p.errout)
	if p.options.SplitFuncStderr != nil {
		out.Split(p.options.SplitFuncStderr)
	}

	for out.Scan() {
		txt := out.Text()
		message := MessagePortError{
			ID:    id,
			Tag:   p.options.Tag,
			Error: fmt.Errorf("%s", txt),
		}
		atomic.AddUint64(&p.bytesIn, uint64(len(txt)))
		if err := p.Send(to, message); err != nil {
			p.Log().Error("unable to send MessagePortError: %s", err)
			return
		}
	}
}
