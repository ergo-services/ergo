package decimal

import (
	"database/sql/driver"
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
)

var ErrWire = errors.New("decimal: malformed binary form")

// WireLen returns the length of the wire form of d.
func (d Decimal) WireLen() int {
	_, m, _ := d.parts()
	return 2 + (bitlen128(m)+7)>>3
}

func (d Decimal) wire(w *[18]byte) int {
	neg, m, s := d.parts()
	n := (bitlen128(m) + 7) >> 3
	binary.BigEndian.PutUint64(w[2:10], m.hi)
	binary.BigEndian.PutUint64(w[10:18], m.lo)
	i := 16 - n
	h := byte(s)
	if neg {
		h |= 0x80
	}
	w[i] = h
	w[i+1] = byte(n)
	return i
}

// PutWire writes the wire form of d into b, which must hold WireLen bytes,
// and returns the number of bytes written.
func (d Decimal) PutWire(b []byte) int {
	var w [18]byte
	i := d.wire(&w)
	return copy(b, w[i:])
}

// AppendBinary appends the wire form of d to b.
func (d Decimal) AppendBinary(b []byte) ([]byte, error) {
	var w [18]byte
	i := d.wire(&w)
	return append(b, w[i:]...), nil
}

// DecodeWire decodes the wire form at the start of b and returns the number
// of bytes it takes.
func DecodeWire(b []byte) (Decimal, int, error) {
	if len(b) < 2 {
		return Decimal{}, 0, ErrWire
	}
	h, n := b[0], int(b[1])
	s := int(h & scaleMask)
	if h&0x40 != 0 || s > MaxScale || n > 16 || len(b) < 2+n {
		return Decimal{}, 0, ErrWire
	}
	var w [16]byte
	copy(w[16-n:], b[2:2+n])
	m := u128{hi: binary.BigEndian.Uint64(w[0:8]), lo: binary.BigEndian.Uint64(w[8:16])}
	neg := h&0x80 != 0
	if fits(neg, m) == false {
		return Decimal{}, 0, ErrWire
	}
	return pack(neg, m, s), 2 + n, nil
}

// MarshalBinary implements encoding.BinaryMarshaler with the wire form.
func (d Decimal) MarshalBinary() ([]byte, error) {
	var w [18]byte
	i := d.wire(&w)
	return append(make([]byte, 0, 18-i), w[i:]...), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (d *Decimal) UnmarshalBinary(b []byte) error {
	v, n, err := DecodeWire(b)
	if err != nil {
		return err
	}
	if n != len(b) {
		return ErrWire
	}
	*d = v
	return nil
}

type extender interface {
	Extend(n int) []byte
}

// MarshalEDF implements the EDF marshaler with the wire form.
func (d Decimal) MarshalEDF(w io.Writer) error {
	if e, ok := w.(extender); ok {
		var b [18]byte
		i := d.wire(&b)
		copy(e.Extend(18-i), b[i:])
		return nil
	}
	b, _ := d.MarshalBinary()
	_, err := w.Write(b)
	return err
}

// UnmarshalEDF implements the EDF unmarshaler.
func (d *Decimal) UnmarshalEDF(b []byte) error {
	return d.UnmarshalBinary(b)
}

// MarshalText implements encoding.TextMarshaler with the String form.
func (d Decimal) MarshalText() ([]byte, error) {
	var b fmtBuf
	i, j := b.format(d, true)
	return append(make([]byte, 0, j-i), b[i:j]...), nil
}

// AppendText implements encoding.TextAppender.
func (d Decimal) AppendText(b []byte) ([]byte, error) {
	return d.AppendString(b), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Decimal) UnmarshalText(b []byte) error {
	v, err := parse(b)
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// MarshalJSON encodes d as a JSON string ("1.5"): JSON numbers are float64
// in most decoders.
func (d Decimal) MarshalJSON() ([]byte, error) {
	var b fmtBuf
	i, j := b.format(d, true)
	out := make([]byte, j-i+2)
	out[0] = '"'
	copy(out[1:], b[i:j])
	out[len(out)-1] = '"'
	return out, nil
}

// UnmarshalJSON accepts a JSON string or number; null leaves d unchanged.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	if len(b) >= 2 && b[0] == '"' && b[len(b)-1] == '"' {
		b = b[1 : len(b)-1]
	}
	v, err := parse(b)
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// Value implements driver.Valuer: the String form.
func (d Decimal) Value() (driver.Value, error) {
	return d.String(), nil
}

// Scan implements sql.Scanner.
func (d *Decimal) Scan(value any) error {
	var err error
	switch v := value.(type) {
	case string:
		*d, err = parse(v)
	case []byte:
		*d, err = parse(v)
	case int64:
		*d = NewFromInt(v)
	case float64:
		*d = NewFromFloat(v)
	case float32:
		*d = NewFromFloat32(v)
	case uint64:
		*d = NewFromUint64(v)
	case int:
		*d = NewFromInt(int64(v))
	case Decimal:
		*d = v
	case encoding.TextMarshaler:
		var b []byte
		if b, err = v.MarshalText(); err == nil {
			*d, err = parse(b)
		}
	default:
		err = fmt.Errorf("decimal: cannot scan %T", value)
	}
	return err
}

// NullDecimal is a Decimal that may be NULL.
type NullDecimal struct {
	Decimal Decimal
	Valid   bool
}

// NewNullDecimal returns a valid NullDecimal.
func NewNullDecimal(d Decimal) NullDecimal {
	return NullDecimal{Decimal: d, Valid: true}
}

// Scan implements sql.Scanner.
func (n *NullDecimal) Scan(value any) error {
	if value == nil {
		n.Decimal, n.Valid = Decimal{}, false
		return nil
	}
	n.Valid = true
	return n.Decimal.Scan(value)
}

// Value implements driver.Valuer.
func (n NullDecimal) Value() (driver.Value, error) {
	if n.Valid == false {
		return nil, nil
	}
	return n.Decimal.String(), nil
}

// MarshalJSON encodes null or the decimal.
func (n NullDecimal) MarshalJSON() ([]byte, error) {
	if n.Valid == false {
		return []byte("null"), nil
	}
	return n.Decimal.MarshalJSON()
}

// UnmarshalJSON decodes null or a decimal.
func (n *NullDecimal) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		n.Decimal, n.Valid = Decimal{}, false
		return nil
	}
	n.Valid = true
	return n.Decimal.UnmarshalJSON(b)
}

// GoString implements fmt.GoStringer.
func (d Decimal) GoString() string {
	return "decimal.RequireFromString(" + strconv.Quote(d.StringScaled()) + ")"
}
