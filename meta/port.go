package meta

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"sync/atomic"

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

	p := &port{
		command:  options.Cmd,
		args:     options.Args,
		tag:      options.Tag,
		process:  options.Process,
		splitOut: options.SplitFuncOut,
		splitErr: options.SplitFuncErr,
	}

	if options.Binary.Enable == false {
		return p, nil
	}

	// check sync.Pool
	if options.Binary.ReadBufferPool != nil {
		b := options.Binary.ReadBufferPool.Get()
		if _, ok := b.([]byte); ok == false {
			return nil, fmt.Errorf("ReadBufferPool must be a pool of []byte values")
		}
		// get it back to the pool
		options.Binary.ReadBufferPool.Put(b)
	}

	if options.Binary.ReadBufferSize < 1 {
		options.Binary.ReadBufferSize = defaultBufferSize
	}

	if options.Binary.WriteBufferKeepAlive != nil {
		if options.Binary.WriteBufferKeepAlivePeriod == 0 {
			return nil, fmt.Errorf("enabled KeepAlive options with zero Period")
		}
	}

	if options.Binary.ChunkFixedLength == 0 {
		// dynamic length
		if options.Binary.ChunkHeaderSize == 0 {
			return nil, fmt.Errorf("ChunkHeaderSize must be defined for dynamic chunk size")
		}

		if options.Binary.ChunkHeaderLengthSize+options.Binary.ChunkHeaderLengthPosition > options.Binary.ChunkHeaderSize {
			return nil, fmt.Errorf("ChunkHeaderLengthPosition + ...LengthSize is out of ChunkHeaderSize bounds")
		}

		switch options.Binary.ChunkHeaderLengthSize {
		case 1, 2, 4:
		default:
			return nil, fmt.Errorf("ChunkHeaderLengthSize must be either: 1, 2, or 4 bytes")
		}
	}
	p.binary = options.Binary

	return p, nil
}

type port struct {
	gen.MetaProcess
	tag      string
	process  gen.Atom
	splitOut bufio.SplitFunc
	splitErr bufio.SplitFunc
	binary   PortBinaryOptions
	bytesIn  uint64
	bytesOut uint64
	command  string
	args     []string

	cmd    *exec.Cmd
	in     io.WriteCloser
	out    io.ReadCloser
	errout io.ReadCloser
}

func (p *port) Init(process gen.MetaProcess) error {
	var err error

	p.MetaProcess = process
	p.cmd = exec.Command(p.command, p.args...)

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

	if p.binary.EnableWriteBuffer {
		type wrapCloser struct {
			io.Writer
			io.Closer
		}
		sc := &wrapCloser{
			Closer: p.in,
		}

		if p.binary.WriteBufferKeepAlive != nil {
			sc.Writer = lib.NewFlusherWithKeepAlive(p.in, p.binary.WriteBufferKeepAlive, p.binary.WriteBufferKeepAlivePeriod)
		} else {
			sc.Writer = lib.NewFlusher(p.in)
		}

		p.in = sc
	}

	if p.process == "" {
		to = p.Parent()
	} else {
		to = p.process
	}

	defer func() {
		p.cmd.Process.Kill()
		message := MessagePortTerminate{
			ID:  p.ID(),
			Tag: p.tag,
		}
		if err := p.Send(to, message); err != nil {
			// gen.ErrNotAllowed means parent process was terminated
			p.Log().Trace("unable to send MessagePortTerminate to %s: %s", to, err)
			return
		}
	}()

	message := MessagePortStart{
		ID:  p.ID(),
		Tag: p.tag,
	}
	if err := p.Send(to, message); err != nil {
		p.Log().Error("unable to send MessagePortStart to %v: %s", to, err)
		return err
	}

	// run stderr reader
	go p.readStderr(to)

	if p.binary.Enable {
		return p.readStdoutData(to)
	}
	return p.readStdoutText(to)
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
		if p.binary.ReadBufferPool != nil {
			p.binary.ReadBufferPool.Put(m.Data)
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
	if reason == nil || reason == gen.TerminateReasonNormal {
		return
	}
	p.Log().Error("terminated abnormaly: %s", reason)
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

func (p *port) readStdoutData(to any) error {
	var buf []byte
	var chunk []byte

	id := p.ID()

	buf = make([]byte, p.binary.ReadBufferSize)

	if p.binary.ReadBufferPool == nil {
		chunk = make([]byte, 0, p.binary.ReadBufferSize)
	} else {
		chunk = p.binary.ReadBufferPool.Get().([]byte)
		chunk = chunk[:0]
	}

	cl := p.binary.ChunkFixedLength // chunk length

	for {

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
			continue
		}

		atomic.AddUint64(&p.bytesIn, uint64(n))

		if p.binary.EnableAutoChunk == false {
			// send chunk
			message := MessagePortData{
				ID:   id,
				Tag:  p.tag,
				Data: buf[:n],
			}

			if err := p.Send(to, message); err != nil {
				p.Log().Error("unable to send MessagePort: %s", err)
				return err
			}

			if p.binary.ReadBufferPool == nil {
				buf = make([]byte, p.binary.ReadBufferSize)
				continue
			}

			if buf = p.binary.ReadBufferPool.Get().([]byte); len(buf) == 0 {
				buf = make([]byte, p.binary.ReadBufferSize)
			}
			continue
		}

		// chunking...
		chunk = append(chunk, buf[:n]...)

	next:

		// read length value for the chunk
		if cl == 0 {
			// check if we got the header
			if len(chunk) < p.binary.ChunkHeaderSize {
				continue
			}

			pos := p.binary.ChunkHeaderLengthPosition
			switch p.binary.ChunkHeaderLengthSize {
			case 1:
				cl = int(chunk[pos])
			case 2:
				cl = int(binary.BigEndian.Uint16(chunk[pos : pos+2]))
			case 4:
				cl = int(binary.BigEndian.Uint32(chunk[pos : pos+4]))
			default:
				// shouldn't reach this code
				panic("bug")
			}

			if p.binary.ChunkHeaderLengthIncludesHeader == false {
				cl += p.binary.ChunkHeaderSize
			}

			if p.binary.ChunkMaxLength > 0 {
				if cl > p.binary.ChunkMaxLength {
					p.Log().Error("chunk size %d is exceeded the limit (ChumkMaxLenth: %d)", cl, p.binary.ChunkMaxLength)
					return gen.ErrTooLarge
				}
			}

		}

		if len(chunk) < cl {
			continue
		}

		// send chunk
		message := MessagePortData{
			ID:   id,
			Tag:  p.tag,
			Data: chunk[:cl],
		}

		if err := p.Send(to, message); err != nil {
			p.Log().Error("unable to send MessagePort: %s", err)
			return err
		}

		tail := chunk[cl:]

		// prepare next chunk
		if p.binary.ReadBufferPool == nil {
			chunk = make([]byte, 0, p.binary.ChunkFixedLength)
		} else {
			chunk = p.binary.ReadBufferPool.Get().([]byte)
			chunk = chunk[:0]
		}

		if p.binary.ChunkFixedLength == 0 {
			cl = 0
		}

		if len(tail) > 0 {
			chunk = append(chunk, tail...)
			goto next
		}

	}
}

func (p *port) readStdoutText(to any) error {
	id := p.ID()
	out := bufio.NewScanner(p.out)
	if p.splitOut != nil {
		out.Split(p.splitOut)
	}

	for out.Scan() {
		txt := out.Text()
		message := MessagePortText{
			ID:   id,
			Tag:  p.tag,
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
	if p.splitErr != nil {
		out.Split(p.splitErr)
	}

	for out.Scan() {
		txt := out.Text()
		message := MessagePortError{
			ID:    id,
			Tag:   p.tag,
			Error: fmt.Errorf(txt),
		}
		atomic.AddUint64(&p.bytesIn, uint64(len(txt)))
		if err := p.Send(to, message); err != nil {
			p.Log().Error("unable to send MessagePortError: %s", err)
			return
		}
	}
}
