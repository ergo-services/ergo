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

	if options.Binary.Enable == true {
		// check sync.Pool
		if options.Binary.ReadBufferPool != nil {
			b := options.Binary.ReadBufferPool.Get()
			if _, ok := b.([]byte); ok == false {
				return nil, fmt.Errorf("options.BufferPool must be pool of []byte values")
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
				return nil, fmt.Errorf("option ChunkHeaderSize must be defined for dynamic chunk size")
			}

			if options.Binary.ChunkHeaderLengthSize+options.Binary.ChunkHeaderLengthPosition > options.Binary.ChunkHeaderSize {
				return nil, fmt.Errorf("option ChunkHeaderLengthPosition + ...LengthSize is out of ChunkHeaderSize bounds")
			}

			switch options.Binary.ChunkHeaderLengthSize {
			case 1:
			case 2:
			case 4:
			default:
				return nil, fmt.Errorf("option ChunkHeaderLengthSize must be either: 1, 2, or 4 bytes")
			}
		}
	}

	p := &port{
		command: options.Cmd,
		args:    options.Args,
		tag:     options.Tag,
		process: options.Process,
		binary:  options.Binary,
	}
	return p, nil
}

type port struct {
	gen.MetaProcess
	tag      string
	process  gen.Atom
	binary   PortBinaryOptions
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
	var to any

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

	if p.binary.WriteBuffer {
		if p.binary.WriteBufferKeepAlive != nil {
			p.in = lib.NewFlusherWithKeepAlive(p.in, p.binary.WriteBufferKeepAlive, p.binary.WriteBufferKeepAlivePeriod)
		} else {
			p.in = lib.NewFlusher(p.in)
		}
	}

	if p.process == "" {
		to = p.Parent()
	} else {
		to = p.process
	}

	defer func() {
		p.cmd.Process.Kill()
		message := MessagePortTerminated{
			ID:  p.ID(),
			Tag: p.tag,
		}
		if err := p.Send(to, message); err != nil {
			p.Log().Error("unable to send MessagePortTerminated to %s: %s", to, err)
			return
		}
	}()

	message := MessagePortStarted{
		ID:  p.ID(),
		Tag: p.tag,
	}
	if err := p.Send(to, message); err != nil {
		p.Log().Error("unable to send MessagePortStarted to %v: %s", to, err)
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

func (p *port) readStdoutData(to any) error {
	var buf []byte
	var chunk []byte

	id := p.ID()

	buf = make([]byte, p.binary.ReadBufferSize)

	if p.binary.ReadBufferPool == nil {
		chunk = make([]byte, 0, p.binary.ReadBufferSize)
	} else {
		chunk = p.binary.ReadBufferPool.Get().([]byte)
	}

	l := 0                          // current chunk length
	le := p.binary.ChunkFixedLength // expecting chunk length

	for {

		fmt.Println("...read", len(buf))
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

		l += n

	next:
		if l < le {
			// need more data
			chunk = append(chunk, buf[:n]...)
			continue
		}

		if p.binary.ChunkFixedLength > 0 {

			tail := l - le
			chunk = append(chunk, buf[:le]...)
			fmt.Println("l le", l, le, buf[:le])

			// send chunk
			message := MessagePortData{
				ID:   id,
				Tag:  p.tag,
				Data: chunk,
			}
			atomic.AddUint64(&p.bytesIn, uint64(n))

			if err := p.Send(to, message); err != nil {
				p.Log().Error("unable to send MessagePort: %s", err)
				return err
			}

			// prepare next chunk
			if p.binary.ReadBufferPool == nil {
				chunk = make([]byte, 0, p.binary.ChunkFixedLength)
			} else {
				chunk = p.binary.ReadBufferPool.Get().([]byte)
			}

			if tail == 0 {
				l = 0
				continue
			}

			copy(buf, buf[tail:])
			l = tail
			goto next
		}

		chunk = append(chunk, buf[:n]...)
		// check if we got the header
		if l < p.binary.ChunkHeaderSize {
			continue
		}

		// read length value for the chunk
		if le == 0 {
			pos := p.binary.ChunkHeaderLengthPosition
			switch p.binary.ChunkHeaderLengthSize {
			case 1:
				le = int(chunk[pos])
			case 2:
				le = int(binary.BigEndian.Uint16(chunk[pos : pos+2]))
			case 4:
				le = int(binary.BigEndian.Uint32(chunk[pos : pos+4]))
			default:
				// shouldn't reach this code
				panic("bug")
			}

			if p.binary.ChunkMaxLength > 0 {
				if le > p.binary.ChunkMaxLength {
					p.Log().Error("chunk size %d is exceeded the limit (ChumkMaxLenth: %d)", le, p.binary.ChunkMaxLength)
					return gen.ErrTooLarge
				}
			}
		}

		if l < le {
			continue
		}

		fmt.Printf("LEN chunk %d LEN buf %d ChunkHeaderSize %d", len(chunk), len(buf), p.binary.ChunkHeaderSize)
		if len(chunk)+len(buf) < p.binary.ChunkHeaderSize {
			chunk = append(chunk, buf[:n]...)
			continue
		}
	}
}

func (p *port) readStdoutText(to any) error {
	id := p.ID()
	out := bufio.NewScanner(p.out)

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
