package etf

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/big"
)

type ErrUnknownTerm struct {
	termType byte
}

var (
	ErrFloatScan = fmt.Errorf("read: failed to sscanf float")
	be           = binary.BigEndian
	bTrue        = []byte("true")
	bFalse       = []byte("false")
)

func (c *Context) ReadDist(r io.Reader) (err error) {
	b := make([]byte, 1)
	_, err = io.ReadFull(r, b)
	if err != nil {
		return
	}

	if b[0] != EtDist {
		err = fmt.Errorf("Not dist header: %d", b[0])
		return
	}

	_, err = io.ReadFull(r, b)
	if err != nil {
		return
	}

	refsNum := int(b[0])
	if refsNum > 0 {
		b = make([]byte, (refsNum/2)+1)
		_, err = io.ReadFull(r, b)
		if err != nil {
			return
		}

		flags := make([]cacheFlag, refsNum)
		longAtoms := false

		for i := 0; i < (refsNum + 1); i++ {
			var v byte
			if (i & 0x01) == 0 {
				v = b[i/2] & 0x0F
			} else {
				v = (b[i/2] >> 4) & 0x0F
			}
			if i < refsNum {
				flags[i] = cacheFlag{(v & 0x08) == 0x08, v & 0x07}
			} else {
				longAtoms = (v & 0x01) == 0x01
			}
		}

		var headLen int
		if longAtoms {
			headLen = 1 + 2
		} else {
			headLen = 1 + 1
		}
		currentAtomCache := make([]*string, len(flags))
		i := 0

		for _, _ = range flags {
			if flags[i].isNew {
				b = make([]byte, headLen)
				_, err = io.ReadFull(r, b)
				if err != nil {
					return
				}
				intRef := uint8(b[0])

				var atomLen uint
				if longAtoms {
					atomLen = uint(binary.BigEndian.Uint16(b[1:3]))
				} else {
					atomLen = uint(b[1])
				}
				b = make([]byte, atomLen)
				_, err = io.ReadFull(r, b)
				if err != nil {
					return
				}
				strText := string(b)
				currentAtomCache[i] = &strText

				cIdx := ((uint16(flags[i].segmentIdx) << 8) | uint16(intRef))
				c.atomCache[cIdx] = &strText
			} else {
				b = make([]byte, 1)
				_, err = io.ReadFull(r, b)
				if err != nil {
					return
				}
				intRef := uint8(b[0])
				cIdx := ((uint16(flags[i].segmentIdx) << 8) | uint16(intRef))
				currentAtomCache[i] = c.atomCache[cIdx]
			}
			i++
		}

		c.currentCache = currentAtomCache
	}
	return
}

type Decoder struct {
	context *Context
	r       io.Reader
	buf     []byte
}

func (c *Context) NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		context: c,
		r:       r,
	}
}

func (c *Context) Read(r io.Reader) (term Term, err error) {
	reader := &Decoder{
		context: c,
		r:       r,
	}
	return reader.NextTerm()
}

func (d *Decoder) NextTerm() (term Term, err error) {
	etype, err := d.readByte()
	if err != nil {
		return nil, err
	}
	var b []byte

	switch etype {
	case ettAtom, ettAtomUTF8:
		// $dLL… | $vLL…
		b, err = d.uint16BorrowRead()
		if err != nil {
			break
		}
		term = Atom(b)

	case ettSmallAtom, ettSmallAtomUTF8:
		// $sL…, $wL…
		b, err = d.uint8BorrowRead()
		if err != nil {
			break
		}
		term = Atom(b)

	case ettBinary:
		// $mLLLL…
		if d.context.ConvertBinaryToString {
			b, err = d.uint32BorrowRead()
			if err != nil {
				break
			}

			term = string(b)
		} else {
			if b, err = d.buint32(); err == nil {
				_, err = io.ReadFull(d.r, b)
				term = b
			}
		}

	case ettString:
		// $kLL…
		if b, err = d.buint16(); err == nil {
			_, err = io.ReadFull(d.r, b)
			term = string(b)
		}

	case ettFloat:
		// $cFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF0
		b, err = d.read(31)
		if err != nil {
			break
		}

		var r int
		var f float64
		if r, err = fmt.Sscanf(string(b), "%f", &f); r != 1 && err == nil {
			err = ErrFloatScan
		}
		term = f

	case ettNewFloat:
		// $FFFFFFFFF
		b, err = d.read(8)
		if err != nil {
			break
		}

		term = math.Float64frombits(be.Uint64(b))

	case ettSmallInteger:
		// $aI
		var x uint8
		x, err = d.readByte()
		term = int(x)

	case ettInteger:
		// $bIIII
		// var x int32
		// b, err := d.read(4)
		// if err != nil {
		// 	return nil, err
		// }
		// ux := be.Uint32(b)
		// x := int(ux >> 1)
		// if ux&1 != 0 {
		// 	x ^= x
		// }
		// term = int(x)
		var x int32
		err = binary.Read(d.r, be, &x)
		if err != nil {
			break
		}
		term = int(x)

	case ettSmallBig:
		// $nAS…
		b, err = d.read(2)
		if err != nil {
			break
		}
		sign := b[1]
		term, err = d.readBigInt(int(b[0]), sign)

	case ettLargeBig:
		// $oAAAAS…
		b, err = d.read(5)
		if err != nil {
			break
		}

		sign := b[4]
		term, err = d.readBigInt(int(be.Uint32(b[:4])), sign)

	case ettNil:
		// $j
		term = List{}

	case ettPid:
		var node interface{}
		var pid Pid
		b = make([]byte, 9)
		if node, err = d.NextTerm(); err != nil {
			return
		} else if _, err = io.ReadFull(d.r, b); err != nil {
			return
		}
		pid.Node = node.(Atom)
		pid.Id = be.Uint32(b[:4])
		pid.Serial = be.Uint32(b[4:8])
		pid.Creation = b[8]
		term = pid

	case ettNewRef:
		// $rLL…
		var ref Ref
		var node interface{}
		var nid uint16
		if nid, err = d.ruint16(); err != nil {
			return
		} else if node, err = d.NextTerm(); err != nil {
			return
		} else if ref.Creation, err = d.readByte(); err != nil {
			return
		}
		ref.Node = node.(Atom)
		ref.Id = make([]uint32, nid)
		for i := 0; i < cap(ref.Id); i++ {
			if ref.Id[i], err = d.ruint32(); err != nil {
				return
			}
		}
		term = ref

	case ettRef:
		// $e…LLLLB
		var ref Ref
		var node interface{}
		if node, err = d.NextTerm(); err != nil {
			return
		}
		ref.Node = node.(Atom)
		ref.Id = make([]uint32, 1)
		if ref.Id[0], err = d.ruint32(); err != nil {
			return
		} else if _, err = io.ReadFull(d.r, b); err != nil {
			return
		}
		ref.Creation = b[0]
		term = ref

	case ettSmallTuple:
		// $hA…
		var arity uint8
		if arity, err = d.readByte(); err != nil {
			break
		}
		tuple := make(Tuple, arity)
		for i := 0; i < cap(tuple); i++ {
			if tuple[i], err = d.NextTerm(); err != nil {
				break
			}
		}
		term = tuple

	case ettLargeTuple:
		// $iAAAA…
		var arity uint32
		if arity, err = d.ruint32(); err != nil {
			break
		}
		tuple := make(Tuple, arity)
		for i := 0; i < cap(tuple); i++ {
			if tuple[i], err = d.NextTerm(); err != nil {
				break
			}
		}
		term = tuple

	case ettList:
		// $lLLLL…$j
		var n uint32
		if n, err = d.ruint32(); err != nil {
			return
		}

		list := make(List, n+1)
		for i := 0; i < cap(list); i++ {
			if list[i], err = d.NextTerm(); err != nil {
				return
			}
		}

		switch list[n].(type) {
		case List:
			// proper list, remove nil element
			list = list[:n]
		}
		term = list

	case ettMap:
		// $mLLLL...
		var n uint32
		if n, err = d.ruint32(); err != nil {
			return
		}

		mp := make(Map, n)
		for i := uint32(0); i < n; i++ {
			var key Term
			if key, err = d.NextTerm(); err != nil {
				return nil, err
			}

			var value Term
			if value, err = d.NextTerm(); err != nil {
				return nil, err
			}

			mp[key] = value
		}
		term = mp

	case ettBitBinary:
		// $MLLLLB…
		var length uint32
		var bits uint8
		if length, err = d.ruint32(); err != nil {
			break
		} else if bits, err = d.readByte(); err != nil {
			break
		}
		b := make([]byte, length)
		_, err = io.ReadFull(d.r, b)
		b[len(b)-1] = b[len(b)-1] >> (8 - bits)
		term = b

	case ettExport:
		// $qM…F…A
		var m, f interface{}
		var a uint8
		if m, err = d.NextTerm(); err != nil {
			break
		} else if f, err = d.NextTerm(); err != nil {
			break
		} else if a, err = d.readByte(); err != nil {
			break
		}

		term = Export{m.(Atom), f.(Atom), a}

	case ettNewFun:
		// $pSSSSAUUUUUUUUUUUUUUUUIIIIFFFFM…i…u…P…[V…]
		var f Function
		d.ruint32()
		f.Arity, _ = d.readByte()
		io.ReadFull(d.r, f.Unique[:])
		f.Index, _ = d.ruint32()
		f.Free, _ = d.ruint32()
		m, _ := d.NextTerm()
		oldi, _ := d.NextTerm()
		oldu, _ := d.NextTerm()
		pid, _ := d.NextTerm()

		f.FreeVars = make([]Term, f.Free)
		for i := 0; i < cap(f.FreeVars); i++ {
			if f.FreeVars[i], err = d.NextTerm(); err != nil {
				break
			}
		}

		f.Module = m.(Atom)
		f.OldIndex = uint32(oldi.(int))
		f.OldUnique = uint32(oldu.(int))
		f.Pid = pid.(Pid)
		term = f

	case ettFun:
		// $uFFFFP…M…i…u…[V…]
		var f Function
		f.Free, _ = d.ruint32()
		pid, _ := d.NextTerm()
		m, _ := d.NextTerm()
		oldi, _ := d.NextTerm()
		oldu, _ := d.NextTerm()

		f.FreeVars = make([]Term, f.Free)
		for i := 0; i < cap(f.FreeVars); i++ {
			if f.FreeVars[i], err = d.NextTerm(); err != nil {
				break
			}
		}

		f.Module = m.(Atom)
		f.OldIndex = uint32(oldi.(int))
		f.OldUnique = uint32(oldu.(int))
		f.Pid = pid.(Pid)
		term = f

	case ettPort:
		// $fA…IIIIC
		var p Port
		a, _ := d.NextTerm()
		p.Node = a.(Atom)
		p.Id, _ = d.ruint32()
		p.Creation, err = d.readByte()
		term = p

	case ettCacheRef:
		b = make([]byte, 1)
		if _, err = io.ReadFull(d.r, b); err != nil {
			break
		}
		term = Atom(*d.context.currentCache[b[0]])

	default:
		err = &ErrUnknownTerm{etype}
	}

	return
}

// Reuses slice so be careful not to hold on to the returned slice
func (d *Decoder) read(n int) ([]byte, error) {
	if len(d.buf) < n {
		d.buf = make([]byte, n)
	}

	sub := d.buf[:n]
	_, err := io.ReadFull(d.r, sub)
	return sub, err

}

func (d *Decoder) readByte() (byte, error) {
	b, err := d.read(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (e *ErrUnknownTerm) Error() string {
	return fmt.Sprintf("read: unknown term type %d", e.termType)
}

var (
	biggestInt = big.NewInt(0xfffffffffffffff)
	lowestInt  = big.NewInt(-0xfffffffffffffff)
)

func (d *Decoder) readBigInt(l int, sign byte) (interface{}, error) {
	b, err := d.read(l)
	if err != nil {
		return nil, err
	}
	if l <= 8 {
		r := d.readInt64(b, sign)
		return r, nil
	}

	hsize := l >> 1
	for i := 0; i < hsize; i++ {
		b[i], b[l-i-1] = b[l-i-1], b[i]
	}

	v := new(big.Int).SetBytes(b)
	if sign != 0 {
		v = v.Neg(v)
	}

	// try int and int64
	if v.Cmp(biggestInt) < 0 && v.Cmp(lowestInt) > 0 {
		return v.Int64(), nil
	}

	return v, nil
}

var powers = []int64{
	int64(math.Pow(256, 0)),
	int64(math.Pow(256, 1)),
	int64(math.Pow(256, 2)),
	int64(math.Pow(256, 3)),
	int64(math.Pow(256, 4)),
	int64(math.Pow(256, 5)),
	int64(math.Pow(256, 6)),
	int64(math.Pow(256, 7)),
}

func (d *Decoder) readInt64(b []byte, sign byte) int64 {
	var result int64
	for i := 0; i < len(b); i++ {
		result += int64(b[i]) * powers[i]
	}
	if sign != 0 {
		result = -result
	}
	return result
}

func (d *Decoder) ruint16() (uint16, error) {
	b, err := d.read(2)
	return be.Uint16(b), err
}

func (d *Decoder) ruint32() (uint32, error) {
	b, err := d.read(4)
	return be.Uint32(b), err
}

// reads a byte and makes a byte slice with that size
func (d *Decoder) buint8() ([]byte, error) {
	size, err := d.readByte()
	return make([]byte, size), err
}

// reads a byte then reads n amount of bytes into a slice
// the returned slice is reused so do not keep it around
func (d *Decoder) uint8BorrowRead() ([]byte, error) {
	size, err := d.readByte()
	if err != nil {
		return nil, err
	}
	return d.read(int(size))
}

// reads 4 bytes as an uint and makes a byte slice with that size
func (d *Decoder) buint16() ([]byte, error) {
	size, err := d.ruint16()
	return make([]byte, size), err
}

// reads 2 bytes then reads n amount of bytes into a slice
// the returned slice is reused so do not keep it around
func (d *Decoder) uint16BorrowRead() ([]byte, error) {
	size, err := d.ruint16()
	if err != nil {
		return nil, err
	}

	return d.read(int(size))
}

// reads 4 bytes as an uint and makes a byte slice with that size
func (d *Decoder) buint32() ([]byte, error) {
	size, err := d.ruint32()
	return make([]byte, size), err
}

// reads 4 bytes then reads n amount of bytes into a slice
// the returned slice is reused so do not keep it around
func (d *Decoder) uint32BorrowRead() ([]byte, error) {
	size, err := d.ruint32()
	if err != nil {
		return nil, err
	}

	return d.read(int(size))
}
